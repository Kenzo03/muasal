package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var (
	errNodeCycle = errors.New("a node cannot move under itself")
	errBadParent = errors.New("the parent is not a live node of this project")
)

// ListNodes returns the project's tree as a flat list with parent ids (FSD §7.1).
func (s *Server) ListNodes(w http.ResponseWriter, r *http.Request, key string, params ListNodesParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListNodes(r.Context(), db.ListNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		IncludeArchived: deref(params.Archived) && pc.scope.Allows(access.Admin), // archived nodes are for restoring (R-MR-4)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Node, len(rows))
	for i, n := range rows {
		items[i] = toAPINodeRow(n)
	}
	writeJSON(w, http.StatusOK, NodeList{Items: items})
}

func (s *Server) CreateNode(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in NodeCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	aliases, aliasesOK := cleanAliases(deref(in.Aliases))
	specific := deref(in.ClientSpecific)
	fields := validateNode(&in.Name, &in.Type, in.Code, aliasesOK, in.Description)
	fields = append(fields, clientScopeErrors(string(in.Type), specific, in.ClientSpecific, in.ClientIds)...)
	if in.ParentId != nil {
		parent, err := s.q.GetNode(ctx, *in.ParentId)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.fail(w, r, err)
			return
		}
		if err != nil || parent.ProjectID != pc.project.ID || parent.ArchivedAt != nil {
			fields = append(fields, FieldError{Field: "parent_id", Code: "invalid", Message: "Choose a parent in this project"})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Node
	err := s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.CreateNode(ctx, db.CreateNodeParams{
			ProjectID: pc.project.ID, ParentID: in.ParentId, Type: string(in.Type), Name: strings.TrimSpace(in.Name),
			Code: nonEmpty(in.Code), Aliases: aliases, Description: strings.TrimSpace(deref(in.Description)),
			ClientSpecific: specific,
		})
		if err != nil {
			return err
		}
		if specific {
			if err := q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: n.ID, ProjectID: pc.project.ID, ClientIds: *in.ClientIds}); err != nil {
				return err
			}
		}
		clients, err := q.ListNodeClients(ctx, n.ID)
		if err != nil {
			return err
		}
		out = toAPINode(n, clients)
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "node", n.ID, "create", nodeAudit(n, clients))
	})
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, n, ok := s.nodeFor(w, r, id, access.Admin)
	if !ok {
		return
	}
	var in NodeUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	var aliases []string
	aliasesOK := true
	if in.Aliases != nil {
		aliases, aliasesOK = cleanAliases(*in.Aliases)
	}
	typ, specific := n.Type, n.ClientSpecific
	if in.Type != nil {
		typ = string(*in.Type)
	}
	if in.ClientSpecific != nil {
		specific = *in.ClientSpecific
	}
	fields := validateNode(in.Name, in.Type, in.Code, aliasesOK, in.Description)
	fields = append(fields, clientScopeErrors(typ, specific, in.ClientSpecific, in.ClientIds)...)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	if in.Archived != nil && *in.Archived && n.ArchivedAt == nil {
		live, err := s.q.CountLiveChildren(r.Context(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if live > 0 {
			writeProblem(w, http.StatusConflict, "node_has_children", "Archive or move its sub-items first")
			return
		}
	}
	if in.Archived != nil && !*in.Archived && n.ArchivedAt != nil && n.ParentID != nil {
		parent, err := s.q.GetNode(r.Context(), *n.ParentID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if parent.ArchivedAt != nil {
			writeProblem(w, http.StatusConflict, "parent_archived", "Restore its parent first")
			return
		}
	}
	ctx := r.Context()
	var out Node
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		before, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		updated, err := q.UpdateNode(ctx, db.UpdateNodeParams{
			ID: id, Name: trimmed(in.Name), Type: (*string)(in.Type), Code: trimmed(in.Code), Aliases: aliases,
			Description: trimmed(in.Description), ClientSpecific: in.ClientSpecific, Archived: in.Archived,
		})
		if err != nil {
			return err
		}
		if in.ClientSpecific != nil {
			if err := q.ClearNodeClients(ctx, id); err != nil {
				return err
			}
			if *in.ClientSpecific {
				if err := q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: id, ProjectID: n.ProjectID, ClientIds: *in.ClientIds}); err != nil {
					return err
				}
			}
		}
		if in.Move != nil {
			if updated, err = moveNode(ctx, q, updated, in.Move); err != nil {
				return err
			}
		}
		after, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		out = toAPINode(updated, after)
		// Chunks name each menu's path, so a rename or a move re-indexes the tickets and notes below it (§13.1).
		if updated.Name != n.Name || in.Move != nil {
			ids, err := q.ListTicketIDsUnderNode(ctx, id)
			if err != nil {
				return err
			}
			if err := s.index(ctx, tx, ids...); err != nil {
				return err
			}
			notes, err := q.ListNoteIDsUnderNode(ctx, id)
			if err != nil {
				return err
			}
			if err := s.indexNote(ctx, tx, notes...); err != nil {
				return err
			}
		}
		return audit(ctx, q, webMeta(r).inProject(n.ProjectID), &pc.user.ID, "node", id, "update",
			changed(nodeAudit(n, before), nodeAudit(updated, after)))
	})
	if errors.Is(err, errNodeCycle) || errors.Is(err, errBadParent) {
		f := FieldError{Field: "move.parent_id", Code: "invalid", Message: "Choose a parent in this project"}
		if errors.Is(err, errNodeCycle) {
			f.Code, f.Message = "node_cycle", "An item cannot move under itself"
		}
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", f)
		return
	}
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DeleteNode removes a node that has no sub-nodes. Before tickets exist nothing
// links to a node, so every node can be deleted (R-MR-4).
func (s *Server) DeleteNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, n, ok := s.nodeFor(w, r, id, access.Admin)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		clients, err := q.ListNodeClients(ctx, id)
		if err != nil {
			return err
		}
		if err := q.DeleteNode(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(n.ProjectID), &pc.user.ID, "node", id, "delete", nodeAudit(n, clients))
	})
	if nodeConflict(w, err) {
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// nodeFor loads a node for the signed-in user. A node in a project the user
// does not belong to, or one hidden by their client scope, answers 404 like a
// missing one (R-AC-5, R-AC-7); a role below need answers 403.
func (s *Server) nodeFor(w http.ResponseWriter, r *http.Request, id int64, need string) (projectCtx, db.Node, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.Node{}, false
	}
	ctx := r.Context()
	n, err := s.q.GetNode(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Not found")
		return projectCtx{}, db.Node{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	p, err := s.q.GetProjectByID(ctx, n.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.Node{}, false
	}
	visible, err := s.q.IsNodeVisible(ctx, db.IsNodeVisibleParams{ID: id, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, false
	}
	if !visible {
		writeProblem(w, http.StatusNotFound, "not_found", "Not found")
		return projectCtx{}, db.Node{}, false
	}
	if !pc.scope.Allows(need) {
		denyRole(w, pc)
		return projectCtx{}, db.Node{}, false
	}
	return pc, n, true
}

// moveNode puts n under move.parent_id (nil = top level) at move.position
// (default last) and renumbers its new siblings. The project row lock makes
// concurrent moves take turns, so none of them can build a cycle (R-MR-5).
func moveNode(ctx context.Context, q *db.Queries, n db.Node, move *NodeMove) (db.Node, error) {
	if err := q.LockProject(ctx, n.ProjectID); err != nil {
		return n, err
	}
	if move.ParentId != nil {
		parent, err := q.GetNode(ctx, *move.ParentId)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (parent.ProjectID != n.ProjectID || parent.ArchivedAt != nil)) {
			return n, errBadParent
		}
		if err != nil {
			return n, err
		}
		cycle, err := q.IsSelfOrDescendant(ctx, db.IsSelfOrDescendantParams{NodeID: n.ID, CandidateID: parent.ID})
		if err != nil {
			return n, err
		}
		if cycle {
			return n, errNodeCycle
		}
	}
	siblings, err := q.ListSiblingIDs(ctx, db.ListSiblingIDsParams{ProjectID: n.ProjectID, ParentID: move.ParentId, ExcludeID: n.ID})
	if err != nil {
		return n, err
	}
	at := len(siblings)
	if move.Position != nil {
		at = min(max(int(*move.Position), 0), len(siblings))
	}
	if err := q.PlaceNodes(ctx, db.PlaceNodesParams{ParentID: move.ParentId, Ids: slices.Insert(siblings, at, n.ID)}); err != nil {
		return n, err
	}
	return q.GetNode(ctx, n.ID)
}

func validateNode(name *string, typ *NodeType, code *string, aliasesOK bool, description *string) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if typ != nil && !typ.Valid() {
		f = append(f, FieldError{Field: "type", Code: "invalid", Message: "Choose module or menu"})
	}
	if code != nil && len(strings.TrimSpace(*code)) > 100 {
		f = append(f, FieldError{Field: "code", Code: "invalid", Message: "Use at most 100 characters"})
	}
	if !aliasesOK {
		f = append(f, aliasesError)
	}
	if description != nil && len(*description) > 5000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 5,000 characters"})
	}
	return f
}

// clientScopeErrors checks R-MR-7: only menus are client-specific, and a
// request that makes a menu client-specific names at least one client.
func clientScopeErrors(typ string, specific bool, set *bool, ids *[]int64) []FieldError {
	switch {
	case specific && typ == string(NodeTypeModule):
		return []FieldError{{Field: "client_specific", Code: "client_specific_module", Message: "Only menus can be client-specific"}}
	case set != nil && *set && (ids == nil || len(*ids) == 0):
		return []FieldError{{Field: "client_ids", Code: "required", Message: "Choose at least one client"}}
	}
	return nil
}

// nodeConflict answers the constraint errors of node writes and reports
// whether it wrote a response.
func nodeConflict(w http.ResponseWriter, err error) bool {
	switch constraintOf(err) {
	case "nodes_sibling_uq":
		const msg = "Another item under the same parent has this name"
		writeProblem(w, http.StatusConflict, "node_name_taken", msg, FieldError{Field: "name", Code: "node_name_taken", Message: msg})
	case "nodes_code_uq":
		const msg = "Another item in this project has this code"
		writeProblem(w, http.StatusConflict, "node_code_taken", msg, FieldError{Field: "code", Code: "node_code_taken", Message: msg})
	case "node_clients_linked":
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "client_not_linked", Message: "Link this client to the project first"})
	case "ticket_nodes_node_fk":
		writeProblem(w, http.StatusConflict, "node_linked", "Tickets link to this item; archive it instead")
	case "decision_note_nodes_node_id_fkey": // notes are history too (§9.4)
		writeProblem(w, http.StatusConflict, "node_linked", "Decision notes link to this item; archive it instead")
	case "nodes_parent_fk":
		writeProblem(w, http.StatusConflict, "node_has_children", "Delete or move its sub-items first")
	default:
		return false
	}
	return true
}

func toAPINode(n db.Node, clients []db.ListNodeClientsRow) Node {
	out := Node{
		Id: n.ID, ParentId: n.ParentID, Type: NodeType(n.Type), Name: n.Name, Code: n.Code,
		Aliases: orEmpty(n.Aliases), Description: n.Description, ClientSpecific: n.ClientSpecific,
		Clients: make([]NodeClient, len(clients)), Position: n.Position, Archived: n.ArchivedAt != nil,
	}
	for i, c := range clients {
		out.Clients[i] = NodeClient{Id: c.ID, Name: c.Name}
	}
	return out
}

func toAPINodeRow(n db.ListNodesRow) Node {
	clients := make([]NodeClient, len(n.ClientIds))
	for j, id := range n.ClientIds {
		clients[j] = NodeClient{Id: id, Name: n.ClientNames[j]}
	}
	return Node{
		Id: n.ID, ParentId: n.ParentID, Type: NodeType(n.Type), Name: n.Name, Code: n.Code,
		Aliases: orEmpty(n.Aliases), Description: n.Description, ClientSpecific: n.ClientSpecific,
		Clients: clients, Position: n.Position, Archived: n.Archived,
	}
}

func nodeAudit(n db.Node, clients []db.ListNodeClientsRow) map[string]any {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return map[string]any{
		"parent_id": n.ParentID, "position": n.Position, "type": n.Type, "name": n.Name, "code": n.Code,
		"aliases": n.Aliases, "description": n.Description, "client_specific": n.ClientSpecific, "client_ids": ids,
		"archived": n.ArchivedAt != nil,
	}
}

// ListRecentNodes lists the nodes of the caller's own latest tickets, so the
// menu picker can put them first (FSD §8.1).
func (s *Server) ListRecentNodes(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	ids, err := s.q.ListRecentNodes(r.Context(), db.ListRecentNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), ReporterID: pc.user.ID,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, RecentNodes{NodeIds: orEmpty(ids)})
}
