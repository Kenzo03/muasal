package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/ticketimport"
)

// importFileMax is the largest ticket import (FSD §14.2).
const importFileMax = 200 << 20

// mayImport reports whether u imports into the project: system admins
// anywhere, project admins into the projects they run (MSL-49).
func (s *Server) mayImport(ctx context.Context, u *db.User, projectID int64) (bool, error) {
	if u.IsAdmin {
		return true, nil
	}
	m, err := s.q.GetMembership(ctx, db.GetMembershipParams{UserID: u.ID, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil && m.Role == "admin", err
}

// ListImports lists the ticket imports the caller may run, newest first.
func (s *Server) ListImports(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListImportRuns(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := ImportRunList{Items: []ImportRun{}}
	for _, row := range rows {
		if ok, err := s.mayImport(r.Context(), u, row.ImportRun.ProjectID); err != nil {
			s.fail(w, r, err)
			return
		} else if !ok {
			continue
		}
		run, err := toAPIImport(db.GetImportRunRow(row), false)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out.Items = append(out.Items, run)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateImport stores an uploaded file as a new import and dry-runs it.
func (s *Server) CreateImport(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, importFileMax+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", "Send the file as multipart/form-data")
		return
	}
	dir := filepath.Join(s.cfg.AttachmentsDir, "imports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		s.fail(w, r, err)
		return
	}
	var path, name, projectKey, preset, mappingJSON string
	stored := false
	defer func() {
		if path != "" && !stored { // a refused or broken upload leaves no file behind
			_ = os.Remove(path)
		}
	}()
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", "Import at most 200 MB at a time")
			return
		}
		switch part.FormName() {
		case "file":
			path = filepath.Join(dir, rand.Text()+".csv")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			n, err := io.Copy(f, io.LimitReader(part, importFileMax+1))
			f.Close()
			if err != nil || n > importFileMax {
				writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", "Import at most 200 MB at a time")
				return
			}
			name = filepath.Base(part.FileName())
		default:
			v, _ := io.ReadAll(io.LimitReader(part, 64<<10))
			switch part.FormName() {
			case "project_key":
				projectKey = strings.ToUpper(strings.TrimSpace(string(v)))
			case "preset":
				preset = strings.TrimSpace(string(v))
			case "mapping":
				mappingJSON = string(v)
			}
		}
		_ = part.Close()
	}
	if path == "" {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "file", Code: "required", Message: "Choose a CSV file"})
		return
	}
	ctx := r.Context()
	p, err := s.q.GetProjectByKey(ctx, projectKey)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "project_key", Code: "not_found", Message: "No project with this key"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if ok, err := s.mayImport(ctx, u, p.ID); err != nil {
		s.fail(w, r, err)
		return
	} else if !ok {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only system admins and this project's admins import tickets")
		return
	}
	if p.ArchivedAt != nil {
		denyRole(w, projectCtx{project: p})
		return
	}
	m := ticketimport.Mapping{Columns: map[string]string{}}
	if preset == "jira" {
		m = ticketimport.JiraPreset()
	}
	if mappingJSON != "" {
		if err := json.Unmarshal([]byte(mappingJSON), &m); err != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
				FieldError{Field: "mapping", Code: "invalid", Message: "The mapping is not valid JSON"})
			return
		}
	}
	mb, _ := json.Marshal(m)
	run, err := s.q.CreateImportRun(ctx, db.CreateImportRunParams{ProjectID: p.ID, FileName: name, FilePath: path, Mapping: mb, CreatedBy: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	stored = true
	out, err := s.planImport(r, run.ID, m)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// GetImport shows an import with its dry run and progress.
func (s *Server) GetImport(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	row, ok := s.importFor(w, r, u, id)
	if !ok {
		return
	}
	out, err := toAPIImport(row, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// PlanImport dry-runs an import again under a new mapping.
func (s *Server) PlanImport(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in ImportMapping
	if !decodeJSON(w, r, &in) {
		return
	}
	row, ok := s.importFor(w, r, u, id)
	if !ok {
		return
	}
	if st := row.ImportRun.Status; st == "running" || st == "done" {
		writeProblem(w, http.StatusConflict, "import_started", "This import has run; upload the file again to import it again")
		return
	}
	var m ticketimport.Mapping
	b, _ := json.Marshal(in)
	_ = json.Unmarshal(b, &m)
	out, err := s.planImport(r, id, m)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// RunImport starts a dry-run import as a background job.
func (s *Server) RunImport(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	row, ok := s.importFor(w, r, u, id)
	if !ok {
		return
	}
	if row.ImportRun.Status != "dry_run" {
		writeProblem(w, http.StatusConflict, "import_not_ready", "Dry-run the import before running it, once")
		return
	}
	ctx := r.Context()
	p, err := s.q.GetProjectByID(ctx, row.ImportRun.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if p.ArchivedAt != nil { // archived after the upload (MSL-64)
		denyRole(w, projectCtx{project: p})
		return
	}
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		// Progress starts over; the dry run's counts stay.
		var st ticketimport.Stats
		_ = json.Unmarshal(row.ImportRun.Stats, &st)
		st.Done, st.LastLine, st.Tickets, st.Comments = 0, 0, 0, 0
		stats, _ := json.Marshal(st)
		if err := q.SetImportStatus(ctx, db.SetImportStatusParams{ID: id, Status: "running", Stats: stats}); err != nil {
			return err
		}
		if _, err := s.jobs.InsertTx(ctx, tx, ticketimport.ImportTickets{RunID: id}, nil); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(row.ImportRun.ProjectID), &u.ID, "import", id, "run", map[string]any{"file": row.ImportRun.FileName})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	row, ok = s.importFor(w, r, u, id)
	if !ok {
		return
	}
	out, err := toAPIImport(row, true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, out)
}

// importFor loads an import the caller may run; others get 404, as for a
// missing one (MSL-49).
func (s *Server) importFor(w http.ResponseWriter, r *http.Request, u *db.User, id int64) (db.GetImportRunRow, bool) {
	row, err := s.q.GetImportRun(r.Context(), id)
	if err == nil {
		var ok bool
		if ok, err = s.mayImport(r.Context(), u, row.ImportRun.ProjectID); err == nil && !ok {
			err = pgx.ErrNoRows
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Import not found")
		return row, false
	}
	if err != nil {
		s.fail(w, r, err)
		return row, false
	}
	return row, true
}

// planImport dry-runs the file under m and saves the mapping, counts and errors.
func (s *Server) planImport(r *http.Request, id int64, m ticketimport.Mapping) (ImportRun, error) {
	ctx := r.Context()
	row, err := s.q.GetImportRun(ctx, id)
	if err != nil {
		return ImportRun{}, err
	}
	p, err := ticketimport.LoadProject(ctx, s.q, row.ImportRun.ProjectID)
	if err != nil {
		return ImportRun{}, err
	}
	f, err := os.Open(row.ImportRun.FilePath)
	if err != nil {
		return ImportRun{}, err
	}
	defer f.Close()
	st, problems, err := ticketimport.Plan(ctx, s.q, p, f, m)
	if err != nil {
		problems = append(problems, ticketimport.Problem{Message: "The file is not readable CSV: " + err.Error()})
	}
	if problems == nil {
		problems = []ticketimport.Problem{}
	}
	mb, _ := json.Marshal(m)
	sb, _ := json.Marshal(st)
	pb, _ := json.Marshal(problems)
	if err := s.q.SaveImportPlan(ctx, db.SaveImportPlanParams{ID: id, Mapping: mb, Status: "dry_run", Stats: sb, Errors: pb}); err != nil {
		return ImportRun{}, err
	}
	if row, err = s.q.GetImportRun(ctx, id); err != nil {
		return ImportRun{}, err
	}
	return toAPIImport(row, true)
}

// toAPIImport shows an import; with headers, it reads the file's header row.
func toAPIImport(row db.GetImportRunRow, headers bool) (ImportRun, error) {
	r := row.ImportRun
	out := ImportRun{
		Id: r.ID, ProjectKey: row.ProjectKey, FileName: r.FileName, Status: ImportRunStatus(r.Status), Headers: []string{},
		CreatedBy: row.CreatedByName, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt, Errors: []ImportProblem{},
	}
	if err := errors.Join(json.Unmarshal(r.Mapping, &out.Mapping), json.Unmarshal(r.Stats, &out.Stats), json.Unmarshal(r.Errors, &out.Errors)); err != nil {
		return out, err
	}
	if out.Mapping.Columns == nil {
		out.Mapping.Columns = map[string]string{}
	}
	if headers {
		if f, err := os.Open(r.FilePath); err == nil {
			if h, err := ticketimport.Headers(f); err == nil {
				out.Headers = h
			}
			f.Close()
		}
	}
	return out, nil
}
