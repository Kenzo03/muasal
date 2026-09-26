package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/treeimport"
)

// importMaxBytes bounds a tree CSV: 5,000 rows fit well within it.
const importMaxBytes = 5 << 20

// ImportNodes plans a module-tree CSV against the project's tree and, unless
// it is a dry run or a row is wrong, applies it in one transaction (FSD §7.5).
// ponytail: synchronous; a tree of 5,000 rows applies in well under a second,
// so it needs no background job.
func (s *Server) ImportNodes(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, importMaxBytes+1<<16)
	mr, err := r.MultipartReader()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", "Send the file as multipart/form-data")
		return
	}
	var file []byte
	dryRun := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", "Import at most 5 MB at a time")
			return
		}
		switch part.FormName() {
		case "file":
			if file, err = io.ReadAll(io.LimitReader(part, importMaxBytes+1)); err != nil || len(file) > importMaxBytes {
				writeProblem(w, http.StatusRequestEntityTooLarge, "file_too_large", "Import at most 5 MB at a time")
				return
			}
		case "dry_run":
			v, _ := io.ReadAll(io.LimitReader(part, 16))
			dryRun = strings.EqualFold(strings.TrimSpace(string(v)), "true")
		}
		_ = part.Close()
	}
	if file == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "file", Code: "required", Message: "Choose a CSV file"})
		return
	}
	ctx := r.Context()
	rows, problems, err := treeimport.Parse(bytes.NewReader(file))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	existing, clients, err := s.treeFor(ctx, s.q, pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	plan := treeimport.Diff(rows, existing, clientNames(clients))
	plan.Problems = append(problems, plan.Problems...)
	out := importResult(plan)
	if dryRun || len(plan.Problems) > 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.LockProject(ctx, pc.project.ID); err != nil {
			return err
		}
		return s.applyTree(ctx, q, tx, r, pc, plan, clients)
	})
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out.Applied = true
	writeJSON(w, http.StatusOK, out)
}

// treeFor reads the project's whole tree, archived nodes too, with each
// node's path and clients, and the project's clients.
func (s *Server) treeFor(ctx context.Context, q *db.Queries, pc projectCtx) ([]treeimport.Existing, []db.Client, error) {
	nodes, err := q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: true, ClientIds: []int64{}, IncludeArchived: true})
	if err != nil {
		return nil, nil, err
	}
	clients, err := q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true, ClientIds: []int64{}})
	if err != nil {
		return nil, nil, err
	}
	byID := map[int64]db.ListNodesRow{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	var out []treeimport.Existing
	for _, n := range nodes {
		var path []string
		for c, ok := n, true; ok; {
			path = append([]string{c.Name}, path...)
			if c.ParentID == nil {
				break
			}
			c, ok = byID[*c.ParentID]
		}
		out = append(out, treeimport.Existing{
			ID: n.ID, Path: path, Type: n.Type, Code: deref(n.Code), ClientSpecific: n.ClientSpecific,
			Clients: n.ClientNames, Aliases: n.Aliases, Archived: n.Archived,
		})
	}
	return out, clients, nil
}

// applyTree renames and moves the changed nodes, then creates the new ones,
// parents first, each audited as coming from the import; tickets and notes
// under changed nodes are re-indexed, as their chunks name the paths (§13.1).
func (s *Server) applyTree(ctx context.Context, q *db.Queries, tx pgx.Tx, r *http.Request, pc projectCtx, plan treeimport.Plan, clients []db.Client) error {
	meta := webMeta(r).inProject(pc.project.ID)
	meta.via = "import"
	ids := map[string]int64{} // lower-case path → node id, as the import leaves the tree
	existing, _, err := s.treeFor(ctx, q, pc)
	if err != nil {
		return err
	}
	for _, e := range existing {
		if !e.Archived {
			ids[strings.ToLower(treeimport.Show(e.Path))] = e.ID
		}
	}
	parentOf := func(path []string) *int64 {
		if len(path) < 2 {
			return nil
		}
		id := ids[strings.ToLower(treeimport.Show(path[:len(path)-1]))]
		return &id
	}
	byDepth := func(a, b treeimport.Row) int { return len(a.Path) - len(b.Path) }
	changes := slices.Clone(plan.Change)
	slices.SortStableFunc(changes, func(a, b treeimport.Change) int { return byDepth(a.Row, b.Row) })
	var touched []int64
	for _, c := range changes {
		n, err := q.GetNode(ctx, c.ID)
		if err != nil {
			return err
		}
		before, err := q.ListNodeClients(ctx, c.ID)
		if err != nil {
			return err
		}
		row := c.Row
		upd := db.UpdateNodeParams{ID: c.ID, Name: ptr(row.Path[len(row.Path)-1]), Type: ptr(row.Type), ClientSpecific: ptr(row.ClientSpecific)}
		if row.Code != "" {
			upd.Code = ptr(row.Code)
		}
		if len(row.Aliases) > 0 {
			upd.Aliases = row.Aliases
		}
		updated, err := q.UpdateNode(ctx, upd)
		if err != nil {
			return err
		}
		if slices.Contains(c.Fields, "clients") {
			if err := setClients(ctx, q, updated, row.Clients, clients); err != nil {
				return err
			}
		}
		if slices.Contains(c.Fields, "parent") {
			parent := parentOf(row.Path)
			if updated, err = moveNode(ctx, q, updated, &NodeMove{ParentId: parent}); err != nil {
				return err
			}
		}
		ids[strings.ToLower(treeimport.Show(row.Path))] = c.ID
		after, err := q.ListNodeClients(ctx, c.ID)
		if err != nil {
			return err
		}
		if err := audit(ctx, q, meta, &pc.user.ID, "node", c.ID, "update", changed(nodeAudit(n, before), nodeAudit(updated, after))); err != nil {
			return err
		}
		touched = append(touched, c.ID)
	}
	creates := slices.Clone(plan.Create)
	slices.SortStableFunc(creates, byDepth)
	for _, row := range creates {
		p := db.CreateNodeParams{
			ProjectID: pc.project.ID, ParentID: parentOf(row.Path), Type: row.Type, Name: row.Path[len(row.Path)-1],
			Aliases: orEmpty(row.Aliases), ClientSpecific: row.ClientSpecific,
		}
		if row.Code != "" {
			p.Code = ptr(row.Code)
		}
		n, err := q.CreateNode(ctx, p)
		if err != nil {
			return err
		}
		if err := q.MarkNodeSource(ctx, db.MarkNodeSourceParams{ID: n.ID, Source: "csv"}); err != nil {
			return err
		}
		if err := setClients(ctx, q, n, row.Clients, clients); err != nil {
			return err
		}
		ids[strings.ToLower(treeimport.Show(row.Path))] = n.ID
		after, err := q.ListNodeClients(ctx, n.ID)
		if err != nil {
			return err
		}
		if err := audit(ctx, q, meta, &pc.user.ID, "node", n.ID, "create", nodeAudit(n, after)); err != nil {
			return err
		}
	}
	for _, id := range touched {
		tickets, err := q.ListTicketIDsUnderNode(ctx, id)
		if err != nil {
			return err
		}
		notes, err := q.ListNoteIDsUnderNode(ctx, id)
		if err != nil {
			return err
		}
		if err := s.reindexNodes(ctx, tx, tickets, notes); err != nil {
			return err
		}
	}
	return nil
}

// setClients replaces a node's clients with the named project clients.
func setClients(ctx context.Context, q *db.Queries, n db.Node, names []string, clients []db.Client) error {
	if err := q.ClearNodeClients(ctx, n.ID); err != nil {
		return err
	}
	var ids []int64
	for _, name := range names {
		for _, c := range clients {
			if strings.EqualFold(c.Name, name) {
				ids = append(ids, c.ID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: n.ID, ProjectID: n.ProjectID, ClientIds: ids})
}

func clientNames(clients []db.Client) []string {
	out := make([]string, len(clients))
	for i, c := range clients {
		out[i] = c.Name
	}
	return out
}

func importResult(p treeimport.Plan) NodeImportResult {
	out := NodeImportResult{Created: []NodeImportRow{}, Changed: []NodeImportRow{}, Unchanged: p.Unchanged, Missing: []string{}, Problems: []NodeImportProblem{}}
	for _, r := range p.Create {
		out.Created = append(out.Created, NodeImportRow{Line: r.Line, Path: treeimport.Show(r.Path), Type: NodeType(r.Type), Fields: []string{}})
	}
	for _, c := range p.Change {
		out.Changed = append(out.Changed, NodeImportRow{Line: c.Row.Line, Path: treeimport.Show(c.Row.Path), Type: NodeType(c.Row.Type), Fields: c.Fields})
	}
	for _, m := range p.Missing {
		out.Missing = append(out.Missing, treeimport.Show(m))
	}
	for _, pr := range p.Problems {
		np := NodeImportProblem{Line: pr.Line, Code: NodeImportProblemCode(pr.Code), Message: pr.Message}
		if pr.Path != "" {
			np.Path = ptr(pr.Path)
		}
		out.Problems = append(out.Problems, np)
	}
	return out
}
