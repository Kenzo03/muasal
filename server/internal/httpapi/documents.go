package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/docs"
	"github.com/kenzo03/muasal/server/internal/draft"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// documentMaxBytes bounds an uploaded document (FSD §7.7).
const documentMaxBytes = 20 << 20

var documentTypes = map[string]string{
	".pdf":      "application/pdf",
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".md":       "text/markdown",
	".markdown": "text/markdown",
}

// ListDocuments lists the project's documents the caller may see (R-MR-15).
func (s *Server) ListDocuments(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectDocuments(r.Context(), db.ListProjectDocumentsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := DocumentList{Items: make([]DocumentListItem, len(rows))}
	for i, d := range rows {
		out.Items[i] = DocumentListItem{Key: d.Key, Title: d.Title, Filename: d.Filename, UploadedBy: d.UploaderName, CreatedAt: d.CreatedAt,
			SupersededBy: d.SupersededByKey}
		if d.ClientID != nil {
			out.Items[i].Client = &Ref{Id: *d.ClientID, Name: deref(d.ClientName)}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// UploadDocument stores a document and its sections (§7.7): the original
// file, and the Markdown the browser converted it to. Its sections are indexed
// for Ask (R-MR-13).
func (s *Server) UploadDocument(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*documentMaxBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_upload", "Send the file, its Markdown and a title as multipart/form-data")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_upload", "Send the file, its Markdown and a title as multipart/form-data")
		return
	}
	defer file.Close()
	name := filepath.Base(header.Filename)
	ctype, allowed := documentTypes[strings.ToLower(filepath.Ext(name))]
	if !allowed {
		writeProblem(w, http.StatusUnsupportedMediaType, "file_type_not_allowed", "Upload a PDF, DOCX or Markdown file")
		return
	}
	ctx := r.Context()
	title := strings.TrimSpace(r.FormValue("title"))
	markdown := r.FormValue("markdown")
	fields := textRange("title", title, 1, 200, "Give the document a title of up to 200 characters")
	if !utf8.ValidString(markdown) || len(markdown) > documentMaxBytes {
		fields = append(fields, FieldError{Field: "markdown", Code: "invalid", Message: "The converted text is not valid"})
	}
	var clientID *int64
	if v := r.FormValue("client_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		ok := err == nil && pc.scope.Sees(&id)
		if ok {
			ok, err = s.q.IsProjectClient(ctx, db.IsProjectClientParams{ProjectID: pc.project.ID, ClientID: id})
			if err != nil {
				s.fail(w, r, err)
				return
			}
		}
		if !ok {
			fields = append(fields, FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client of this project"})
		}
		clientID = &id
	}
	var older *db.GetDocumentByKeyRow
	if v := strings.ToUpper(strings.TrimSpace(r.FormValue("supersedes"))); v != "" {
		d, err := s.q.GetDocumentByKey(ctx, v)
		if err != nil || d.Document.ProjectID != pc.project.ID {
			fields = append(fields, FieldError{Field: "supersedes", Code: "invalid", Message: "Choose a document of this project"})
		} else {
			older = &d
		}
	}
	sections := docs.Split(markdown, title)
	if len(sections) == 0 {
		fields = append(fields, FieldError{Field: "markdown", Code: "required", Message: "The file has no text Muasal can read"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	sum, size, err := s.storeFile(file, documentMaxBytes)
	if errors.Is(err, errTooLarge) {
		writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", fmt.Sprintf("File is larger than %d MB", documentMaxBytes>>20))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var docKey string
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		n, err := q.NextDocNumber(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		docKey = fmt.Sprintf("%s-DOC%d", pc.project.Key, n)
		d, err := q.CreateDocument(ctx, db.CreateDocumentParams{
			ProjectID: pc.project.ID, Number: n, Key: docKey, Title: title, ClientID: clientID, Filename: name,
			ContentType: ctype, SizeBytes: size, Sha256: sum, Markdown: markdown, UploadedBy: pc.user.ID,
		})
		if err != nil {
			return err
		}
		var ids []int64
		for i, sec := range sections {
			id, err := q.CreateSection(ctx, db.CreateSectionParams{DocumentID: d.ID, Number: sec.Number, Title: cutRunes(sec.Title, 500),
				Level: int32(sec.Level), Position: int32(i), Body: sec.Body})
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		changes := map[string]any{"key": docKey, "title": title, "filename": name, "sections": len(sections)}
		if older != nil {
			if err := q.SupersedeDocument(ctx, db.SupersedeDocumentParams{ID: older.Document.ID, By: &d.ID}); err != nil {
				return err
			}
			if err := q.CarrySectionLinks(ctx, db.CarrySectionLinksParams{OldID: older.Document.ID, NewID: d.ID}); err != nil {
				return err // MSL-14: the new version keeps the old one's menus
			}
			old, err := q.ListDocumentSectionIDs(ctx, older.Document.ID) // their chunks now say "superseded"
			if err != nil {
				return err
			}
			ids = append(ids, old...)
			changes["supersedes"] = older.Document.Key
		}
		if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "document", d.ID, "create", changes); err != nil {
			return err
		}
		return s.indexSections(ctx, tx, ids...)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeDocument(w, r, pc, docKey, http.StatusCreated)
}

// documentFor resolves a document the caller may see (R-MR-15); others are 404.
func (s *Server) documentFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, db.GetDocumentByKeyRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	ctx := r.Context()
	d, err := s.q.GetDocumentByKey(ctx, strings.ToUpper(key))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && d.Document.ArchivedAt != nil) {
		writeProblem(w, http.StatusNotFound, "not_found", "Document not found")
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	p, err := s.q.GetProjectByID(ctx, d.Document.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	if !pc.scope.Sees(d.Document.ClientID) {
		writeProblem(w, http.StatusNotFound, "not_found", "Document not found")
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	if !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, db.GetDocumentByKeyRow{}, false
	}
	return pc, d, true
}

// GetDocument reads a document with its sections, the nodes they produced and its drafts.
func (s *Server) GetDocument(w http.ResponseWriter, r *http.Request, key string) {
	pc, _, ok := s.documentFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	s.writeDocument(w, r, pc, strings.ToUpper(key), http.StatusOK)
}

// UpdateDocument marks a document replaced by a newer one of the project, for
// when its upload did not say so, or current again with null (§7.7). Only a
// current document can replace another, so replacements never loop. Its
// sections are indexed again: their chunks say whether they are history.
func (s *Server) UpdateDocument(w http.ResponseWriter, r *http.Request, key string) {
	pc, d, ok := s.documentFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in DocumentUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	var by *int64
	var newKey *string
	if in.SupersededBy != nil {
		newer, err := s.q.GetDocumentByKey(ctx, strings.ToUpper(strings.TrimSpace(*in.SupersededBy)))
		if err != nil || newer.Document.ProjectID != pc.project.ID || newer.Document.ArchivedAt != nil ||
			newer.Document.ID == d.Document.ID || newer.Document.SupersededBy != nil {
			writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
				FieldError{Field: "superseded_by", Code: "invalid", Message: "Choose another current document of this project"})
			return
		}
		by, newKey = &newer.Document.ID, &newer.Document.Key
	}
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.SetDocumentSupersededBy(ctx, db.SetDocumentSupersededByParams{ID: d.Document.ID, By: by}); err != nil {
			return err
		}
		changes := changed(map[string]any{"superseded_by": d.SupersededByKey}, map[string]any{"superseded_by": newKey})
		if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "document", d.Document.ID, "update", changes); err != nil {
			return err
		}
		ids, err := q.ListDocumentSectionIDs(ctx, d.Document.ID)
		if err != nil {
			return err
		}
		if by != nil { // MSL-14: the newer document keeps this one's menus
			if err := q.CarrySectionLinks(ctx, db.CarrySectionLinksParams{OldID: d.Document.ID, NewID: *by}); err != nil {
				return err
			}
			newer, err := q.ListDocumentSectionIDs(ctx, *by)
			if err != nil {
				return err
			}
			ids = append(ids, newer...)
		}
		return s.indexSections(ctx, tx, ids...)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeDocument(w, r, pc, d.Document.Key, http.StatusOK)
}

func (s *Server) writeDocument(w http.ResponseWriter, r *http.Request, pc projectCtx, key string, status int) {
	ctx := r.Context()
	d, err := s.q.GetDocumentByKey(ctx, key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	doc := d.Document
	out := Document{Key: doc.Key, ProjectKey: pc.project.Key, Title: doc.Title, Filename: doc.Filename, Markdown: doc.Markdown,
		UploadedBy: d.UploaderName, CreatedAt: doc.CreatedAt, SupersededBy: d.SupersededByKey}
	out.Replaces = make([]struct {
		Key   string `json:"key"`
		Title string `json:"title"`
	}, 0)
	if doc.ClientID != nil {
		out.Client = &Ref{Id: *doc.ClientID, Name: deref(d.ClientName)}
	}
	sections, err := s.q.ListSections(ctx, doc.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	links, err := s.q.ListSectionNodes(ctx, doc.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	nodes, err := s.visibleNodes(ctx, pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	names := map[int64]string{}
	for _, n := range nodes {
		names[n.ID] = n.Name
	}
	out.Sections = make([]DocumentSection, len(sections))
	for i, sec := range sections {
		out.Sections[i] = DocumentSection{Number: sec.Number, Title: sec.Title, Level: int(sec.Level), Body: sec.Body, Nodes: []Ref{}}
		for _, l := range links {
			if name, ok := names[l.NodeID]; ok && l.SectionID == sec.ID {
				out.Sections[i].Nodes = append(out.Sections[i].Nodes, Ref{Id: l.NodeID, Name: name})
			}
		}
	}
	replaced, err := s.q.ListReplacedDocuments(ctx, &doc.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, o := range replaced {
		out.Replaces = append(out.Replaces, struct {
			Key   string `json:"key"`
			Title string `json:"title"`
		}{o.Key, o.Title})
	}
	drafts, err := s.q.ListDocumentDrafts(ctx, doc.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out.Drafts = make([]struct {
		CreatedAt time.Time       `json:"created_at"`
		Id        int64           `json:"id"`
		Status    TreeDraftStatus `json:"status"`
	}, len(drafts))
	for i, dr := range drafts {
		out.Drafts[i].Id, out.Drafts[i].Status, out.Drafts[i].CreatedAt = dr.ID, TreeDraftStatus(dr.Status), dr.CreatedAt
	}
	writeJSON(w, status, out)
}

// DownloadDocument serves the original file as a download.
func (s *Server) DownloadDocument(w http.ResponseWriter, r *http.Request, key string) {
	_, d, ok := s.documentFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	f, err := os.Open(s.attachmentPath(d.Document.Sha256))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", d.Document.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.Document.Filename}))
	w.Header().Set("Content-Length", strconv.FormatInt(d.Document.SizeBytes, 10))
	_, _ = io.Copy(w, f)
}

// StartTreeDraft drafts a module tree from a document (§7.7). With AI on it
// runs as a background job (R-MR-12); with AI off the proposal comes from the
// headings at once, and no model is called (R-MR-11, AC-MR-10).
func (s *Server) StartTreeDraft(w http.ResponseWriter, r *http.Request, key string) {
	pc, d, ok := s.documentFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	ctx := r.Context()
	st, err := s.ai.Store.Get(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	useAI := st.Mode != ai.ModeOff
	total := 0
	if useAI {
		rows, err := s.q.ListSections(ctx, d.Document.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		sections := make([]docs.Section, len(rows))
		for i, x := range rows {
			sections[i] = docs.Section{Number: x.Number, Title: x.Title, Level: int(x.Level), Body: x.Body}
		}
		total = draft.TreeParts(st, sections)
	}
	var id int64
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		t, err := q.CreateTreeDraft(ctx, db.CreateTreeDraftParams{ProjectID: pc.project.ID, DocumentID: d.Document.ID, Status: "running",
			Proposal: []byte("[]"), UsedAi: useAI, TotalParts: int32(total), CreatedBy: pc.user.ID})
		if err != nil {
			return err
		}
		id = t.ID
		if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "document", d.Document.ID, "tree_draft_start",
			map[string]any{"draft_id": t.ID, "used_ai": useAI}); err != nil {
			return err
		}
		if !useAI {
			return nil
		}
		_, err = s.jobs.InsertTx(ctx, tx, draft.DraftTree{DraftID: t.ID}, nil)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !useAI {
		if err := draft.RunTreeDraft(ctx, s.pool, s.ai, id); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.writeTreeDraft(w, r, id, http.StatusAccepted)
}

// treeDraftFor resolves a draft for its project's admins; anyone else gets 404.
func (s *Server) treeDraftFor(w http.ResponseWriter, r *http.Request, id int64) (projectCtx, db.GetTreeDraftRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	ctx := r.Context()
	t, err := s.q.GetTreeDraft(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Draft not found")
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	p, err := s.q.GetProjectByID(ctx, t.TreeDraft.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	if !pc.scope.Allows(access.Admin) {
		writeProblem(w, http.StatusNotFound, "not_found", "Draft not found")
		return projectCtx{}, db.GetTreeDraftRow{}, false
	}
	return pc, t, true
}

// GetTreeDraft reads a draft and its progress.
func (s *Server) GetTreeDraft(w http.ResponseWriter, r *http.Request, id int64) {
	if _, _, ok := s.treeDraftFor(w, r, id); !ok {
		return
	}
	s.writeTreeDraft(w, r, id, http.StatusOK)
}

func (s *Server) writeTreeDraft(w http.ResponseWriter, r *http.Request, id int64, status int) {
	t, err := s.q.GetTreeDraft(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := t.TreeDraft
	out := TreeDraft{Id: d.ID, DocumentKey: t.DocumentKey, DocumentTitle: t.DocumentTitle, Status: TreeDraftStatus(d.Status), UsedAi: d.UsedAi,
		DoneParts: int(d.DoneParts), TotalParts: int(d.TotalParts), CreatedAt: d.CreatedAt, Proposal: []TreeDraftNode{}}
	if d.Error != "" {
		out.Error = &d.Error
	}
	_ = json.Unmarshal(d.Proposal, &out.Proposal)
	writeJSON(w, status, out)
}

// SaveTreeDraft keeps the admin's review of a ready draft; the tree does not change.
func (s *Server) SaveTreeDraft(w http.ResponseWriter, r *http.Request, id int64) {
	_, t, ok := s.treeDraftFor(w, r, id)
	if !ok {
		return
	}
	var in SaveTreeDraftJSONBody
	if !decodeJSON(w, r, &in) {
		return
	}
	if t.TreeDraft.Status != "ready" {
		writeProblem(w, http.StatusConflict, "draft_not_ready", "Only a ready draft can be edited")
		return
	}
	nodes, fields := checkProposal(in.Proposal)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	b, _ := json.Marshal(nodes)
	if err := s.q.SaveTreeDraft(r.Context(), db.SaveTreeDraftParams{ID: id, Proposal: b}); err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeTreeDraft(w, r, id, http.StatusOK)
}

// checkProposal validates an edited proposal: unique ids, known parents, no
// cycles, names of 1–200 characters and a module or menu type.
func checkProposal(in []TreeDraftNode) ([]docs.Node, []FieldError) {
	var fields []FieldError
	ids := map[string]bool{}
	out := make([]docs.Node, len(in))
	for i, n := range in {
		name := strings.Join(strings.Fields(n.Name), " ")
		if ids[n.TmpId] || n.TmpId == "" {
			fields = append(fields, FieldError{Field: fmt.Sprintf("proposal[%d].tmp_id", i), Code: "invalid", Message: "Each node needs its own id"})
		}
		ids[n.TmpId] = true
		if l := utf8.RuneCountInString(name); l == 0 || l > 200 {
			fields = append(fields, FieldError{Field: fmt.Sprintf("proposal[%d].name", i), Code: "invalid", Message: "Name the node in 1 to 200 characters"})
		}
		if n.Type != NodeTypeModule && n.Type != NodeTypeMenu {
			fields = append(fields, FieldError{Field: fmt.Sprintf("proposal[%d].type", i), Code: "invalid", Message: "Choose module or menu"})
		}
		out[i] = docs.Node{TmpID: n.TmpId, Parent: n.Parent, Type: string(n.Type), Name: name, Code: cutRunes(strings.TrimSpace(deref(n.Code)), 100), Aliases: orEmpty(n.Aliases),
			Description: cutRunes(strings.TrimSpace(n.Description), 300), Sections: orEmpty(n.Sections), Keep: n.Keep, Exists: n.Exists, Duplicate: deref(n.Duplicate)}
	}
	for i, n := range out {
		if n.Parent != "" && !ids[n.Parent] {
			fields = append(fields, FieldError{Field: fmt.Sprintf("proposal[%d].parent", i), Code: "invalid", Message: "Choose a parent in the draft"})
		}
	}
	paths := docs.Paths(out)
	for i, n := range out {
		if p := paths[n.TmpID]; len(p) == 0 || len(p) > 20 {
			fields = append(fields, FieldError{Field: fmt.Sprintf("proposal[%d].parent", i), Code: "invalid", Message: "A node cannot sit under itself"})
		}
	}
	return out, fields
}

// ApplyTreeDraft creates the ticked nodes the tree lacks, parents first, in
// one transaction with source ai_draft, and links every node's source
// sections to it (§7.7). Nodes the tree has stay unchanged.
func (s *Server) ApplyTreeDraft(w http.ResponseWriter, r *http.Request, id int64) {
	pc, t, ok := s.treeDraftFor(w, r, id)
	if !ok {
		return
	}
	if t.TreeDraft.Status != "ready" {
		writeProblem(w, http.StatusConflict, "draft_not_ready", "Only a ready draft can be applied")
		return
	}
	var nodes []docs.Node
	if err := json.Unmarshal(t.TreeDraft.Proposal, &nodes); err != nil {
		s.fail(w, r, err)
		return
	}
	ctx := r.Context()
	paths := docs.Paths(nodes)
	key := func(p []string) string { return strings.ToLower(strings.Join(p, "\x00")) }
	var created, linked int
	var fields []FieldError
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		existing, err := draft.Tree(ctx, q, pc.project.ID)
		if err != nil {
			return err
		}
		// Codes are unique in a project, archived nodes included (MSL-17).
		coded, err := q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: true, ClientIds: []int64{}, IncludeArchived: true})
		if err != nil {
			return err
		}
		usedCodes := map[string]bool{}
		for _, n := range coded {
			if n.Code != nil {
				usedCodes[*n.Code] = true
			}
		}
		ids := map[string]int64{} // path key → node id
		for _, e := range existing {
			ids[key(e.Path)] = e.ID
		}
		byTmp := map[string]docs.Node{}
		for _, n := range nodes {
			byTmp[n.TmpID] = n
		}
		order := slices.Clone(nodes)
		slices.SortStableFunc(order, func(a, b docs.Node) int { return len(paths[a.TmpID]) - len(paths[b.TmpID]) })
		planned := map[string]bool{}
		for _, n := range order {
			k := key(paths[n.TmpID])
			if _, has := ids[k]; has || !n.Keep {
				continue
			}
			if planned[k] {
				fields = append(fields, FieldError{Field: "proposal", Code: "duplicate", Message: "Two nodes share the path " + strings.Join(paths[n.TmpID], " › ")})
				continue
			}
			planned[k] = true
			if p, ok := byTmp[n.Parent]; n.Parent != "" && ok && !p.Keep && !p.Exists {
				if _, has := ids[key(paths[p.TmpID])]; !has {
					fields = append(fields, FieldError{Field: "proposal", Code: "parent_unticked", Message: "Keep the parent of " + strings.Join(paths[n.TmpID], " › ")})
				}
			}
		}
		if len(fields) > 0 {
			return errValidation
		}
		meta := webMeta(r).inProject(pc.project.ID)
		for _, n := range order {
			path := paths[n.TmpID]
			k := key(path)
			if _, has := ids[k]; has || !n.Keep {
				continue
			}
			var parent *int64
			if len(path) > 1 {
				pid := ids[key(path[:len(path)-1])]
				parent = &pid
			}
			var code *string
			if n.Code != "" && !usedCodes[n.Code] {
				code, usedCodes[n.Code] = &n.Code, true
			}
			node, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: pc.project.ID, ParentID: parent, Type: n.Type, Name: n.Name,
				Code: code, Aliases: orEmpty(n.Aliases), Description: n.Description})
			if err != nil {
				return err
			}
			if err := q.MarkNodeSource(ctx, db.MarkNodeSourceParams{ID: node.ID, Source: "ai_draft"}); err != nil {
				return err
			}
			changes := nodeAudit(node, nil)
			changes["tree_draft"] = id
			if err := audit(ctx, q, meta, &pc.user.ID, "node", node.ID, "create", changes); err != nil {
				return err
			}
			ids[k] = node.ID
			created++
		}
		sections, err := q.ListSections(ctx, t.TreeDraft.DocumentID)
		if err != nil {
			return err
		}
		secIDs := map[string]int64{}
		var all []int64
		for _, sec := range sections {
			secIDs[sec.Number] = sec.ID
			all = append(all, sec.ID)
		}
		for _, n := range nodes {
			nodeID, ok := ids[key(paths[n.TmpID])]
			if !ok {
				continue
			}
			for _, num := range n.Sections {
				if sid, ok := secIDs[num]; ok {
					if err := q.LinkSectionNode(ctx, db.LinkSectionNodeParams{SectionID: sid, NodeID: nodeID}); err != nil {
						return err
					}
					linked++
				}
			}
		}
		if _, err := q.SetTreeDraftStatus(ctx, db.SetTreeDraftStatusParams{ID: id, Status: "applied"}); err != nil {
			return err
		}
		if err := audit(ctx, q, meta, &pc.user.ID, "document", t.TreeDraft.DocumentID, "tree_draft_apply",
			map[string]any{"draft_id": id, "created": created, "linked": linked}); err != nil {
			return err
		}
		return s.indexSections(ctx, tx, all...) // chunks now name their nodes
	})
	if errors.Is(err, errValidation) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "linked": linked})
}

// DiscardTreeDraft drops a ready draft; the tree does not change.
func (s *Server) DiscardTreeDraft(w http.ResponseWriter, r *http.Request, id int64) {
	pc, t, ok := s.treeDraftFor(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.SetTreeDraftStatus(ctx, db.SetTreeDraftStatusParams{ID: id, Status: "discarded"}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "document", t.TreeDraft.DocumentID, "tree_draft_discard",
			map[string]any{"draft_id": id})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusConflict, "draft_not_ready", "Only a ready draft can be discarded")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// indexSections queues sections' re-index in the caller's transaction (§13.2).
func (s *Server) indexSections(ctx context.Context, tx pgx.Tx, ids ...int64) error {
	if len(ids) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(ids))
	for i, id := range ids {
		params[i] = river.InsertManyParams{Args: indexer.IndexSection{SectionID: id}}
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, params)
	return err
}

var errValidation = errors.New("validation failed")

func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
