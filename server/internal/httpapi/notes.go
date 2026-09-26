package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// ListNotes lists a project's decision notes the reader may see, newest
// decision first (FSD §9.4).
func (s *Server) ListNotes(w http.ResponseWriter, r *http.Request, key string, params ListNotesParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectNotes(r.Context(), db.ListProjectNotesParams{
		ProjectID: pc.project.ID, Archived: deref(params.Archived), AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := NoteList{Items: make([]NoteSummary, len(rows))}
	for i, n := range rows {
		out.Items[i] = NoteSummary{Key: n.Key, Title: n.Title, DecidedOn: openapi_types.Date{Time: n.DecidedOn}, Archived: n.ArchivedAt != nil}
		if n.ClientID != nil {
			out.Items[i].Client = &Ref{Id: *n.ClientID, Name: deref(n.ClientName)}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateNote records a decision made outside tickets (DC-3). Members may.
func (s *Server) CreateNote(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in NoteInput
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	v, fields, err := s.checkNote(ctx, pc, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Note
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		number, err := q.NextNoteNumber(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		n, err := q.CreateNote(ctx, db.CreateNoteParams{
			ProjectID: pc.project.ID, Number: number, Key: fmt.Sprintf("%s-DN%d", pc.project.Key, number), Title: v.title,
			DecidedOn: in.DecidedOn.Time, ClientID: in.ClientId, Attendees: v.attendees, Body: v.body, CreatedBy: pc.user.ID,
		})
		if err != nil {
			return err
		}
		if err := s.saveNoteLinks(ctx, q, n.ID, v); err != nil {
			return err
		}
		if out, err = readNote(ctx, q, n.ID, pc); err != nil {
			return err
		}
		if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "note", n.ID, "create", noteAudit(out)); err != nil {
			return err
		}
		return s.indexNote(ctx, tx, n.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) GetNote(w http.ResponseWriter, r *http.Request, noteKey string) {
	pc, row, ok := s.noteFor(w, r, noteKey, access.Viewer)
	if !ok {
		return
	}
	out, err := readNote(r.Context(), s.q, row.DecisionNote.ID, pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// UpdateNote replaces a note's fields. The author and project admins may;
// every edit keeps the old words in the note's history (§9.4).
func (s *Server) UpdateNote(w http.ResponseWriter, r *http.Request, noteKey string) {
	pc, row, ok := s.noteFor(w, r, noteKey, access.Member)
	if !ok {
		return
	}
	if !canEditNote(pc, row.DecisionNote) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only the author and project admins edit a note")
		return
	}
	var in NoteUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	v, fields, err := s.checkNote(ctx, pc, NoteInput{
		Title: in.Title, DecidedOn: in.DecidedOn, ClientId: in.ClientId, Attendees: in.Attendees,
		NodeIds: in.NodeIds, TicketKeys: in.TicketKeys, Body: in.Body,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	id := row.DecisionNote.ID
	var out Note
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		before, err := readNote(ctx, q, id, pc)
		if err != nil {
			return err
		}
		if _, err := q.UpdateNote(ctx, db.UpdateNoteParams{
			ID: id, Title: v.title, DecidedOn: in.DecidedOn.Time, ClientID: in.ClientId, Attendees: v.attendees, Body: v.body,
		}); err != nil {
			return err
		}
		if in.Archived != nil {
			if err := q.ArchiveNote(ctx, db.ArchiveNoteParams{ID: id, Archived: *in.Archived}); err != nil {
				return err
			}
		}
		if err := s.saveNoteLinks(ctx, q, id, v); err != nil {
			return err
		}
		if out, err = readNote(ctx, q, id, pc); err != nil {
			return err
		}
		if d := changed(noteAudit(before), noteAudit(out)); len(d) > 0 {
			if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "note", id, "update", d); err != nil {
				return err
			}
		}
		return s.indexNote(ctx, tx, id)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// noteFor loads a note for the signed-in user. A note in a project they do not
// belong to, or of a client outside their scope, answers 404 like a missing
// one (R-AC-2, R-AC-3).
func (s *Server) noteFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, db.GetNoteByKeyRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	ctx := r.Context()
	row, err := s.q.GetNoteByKey(ctx, strings.ToUpper(key))
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Note not found")
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	p, err := s.q.GetProjectByID(ctx, row.DecisionNote.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	if !pc.scope.Sees(row.DecisionNote.ClientID) {
		writeProblem(w, http.StatusNotFound, "not_found", "Note not found")
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	if !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, db.GetNoteByKeyRow{}, false
	}
	return pc, row, true
}

func canEditNote(pc projectCtx, n db.DecisionNote) bool {
	return pc.scope.Allows(access.Admin) || (pc.scope.Allows(access.Member) && n.CreatedBy == pc.user.ID)
}

// noteValues are a note's checked fields.
type noteValues struct {
	title, attendees, body string
	nodeIDs, ticketIDs     []int64
}

// checkNote validates a note against the project and the author's scope.
func (s *Server) checkNote(ctx context.Context, pc projectCtx, in NoteInput) (noteValues, []FieldError, error) {
	v := noteValues{title: strings.TrimSpace(in.Title), attendees: strings.TrimSpace(deref(in.Attendees)), body: strings.TrimSpace(in.Body)}
	var f []FieldError
	if n := utf8.RuneCountInString(v.title); n < 5 || n > 200 {
		f = append(f, FieldError{Field: "title", Code: "invalid", Message: "Use 5 to 200 characters"})
	}
	if in.DecidedOn.IsZero() {
		f = append(f, FieldError{Field: "decided_on", Code: "required", Message: "Required"})
	}
	if in.ClientId != nil {
		linked, err := s.q.IsProjectClient(ctx, db.IsProjectClientParams{ProjectID: pc.project.ID, ClientID: *in.ClientId})
		if err != nil {
			return v, nil, err
		}
		if !linked {
			f = append(f, clientIDField)
		}
	}
	if !pc.scope.Sees(in.ClientId) && !slices.Contains(f, clientIDField) {
		f = append(f, clientIDField)
	}
	if utf8.RuneCountInString(v.attendees) > 500 {
		f = append(f, FieldError{Field: "attendees", Code: "invalid", Message: "Use at most 500 characters"})
	}
	switch n := utf8.RuneCountInString(v.body); {
	case n == 0:
		f = append(f, FieldError{Field: "body", Code: "required", Message: "Required"})
	case n > 50000:
		f = append(f, FieldError{Field: "body", Code: "invalid", Message: "Use at most 50,000 characters"})
	}
	ids, live, err := s.liveNodeIDs(ctx, pc, in.NodeIds)
	if err != nil {
		return v, nil, err
	}
	switch {
	case len(ids) == 0:
		f = append(f, FieldError{Field: "node_ids", Code: "min_items", Message: "Link at least one menu or module"})
	case !live:
		f = append(f, nodeIDsField)
	}
	v.nodeIDs = ids
	v.ticketIDs = []int64{}
	for _, k := range deref(in.TicketKeys) {
		t, err := s.q.GetTicketByKey(ctx, strings.ToUpper(strings.TrimSpace(k)))
		if errors.Is(err, pgx.ErrNoRows) {
			f = append(f, FieldError{Field: "ticket_keys", Code: "not_found", Message: "No ticket " + k + " that you can see"})
			continue
		}
		if err != nil {
			return v, nil, err
		}
		seen, err := s.q.VisibleTicketIDs(ctx, db.VisibleTicketIDsParams{Ids: []int64{t.Ticket.ID}, IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID})
		if err != nil {
			return v, nil, err
		}
		if len(seen) == 0 {
			f = append(f, FieldError{Field: "ticket_keys", Code: "not_found", Message: "No ticket " + k + " that you can see"})
			continue
		}
		if !slices.Contains(v.ticketIDs, t.Ticket.ID) {
			v.ticketIDs = append(v.ticketIDs, t.Ticket.ID)
		}
	}
	return v, f, nil
}

func (s *Server) saveNoteLinks(ctx context.Context, q *db.Queries, id int64, v noteValues) error {
	if err := q.SetNoteNodes(ctx, db.SetNoteNodesParams{NoteID: id, NodeIds: v.nodeIDs}); err != nil {
		return err
	}
	return q.SetNoteTickets(ctx, db.SetNoteTicketsParams{NoteID: id, TicketIds: v.ticketIDs})
}

// readNote reads a note as pc's user sees it: linked tickets they cannot open
// are left out.
func readNote(ctx context.Context, q *db.Queries, id int64, pc projectCtx) (Note, error) {
	row, err := q.GetNoteByID(ctx, id)
	if err != nil {
		return Note{}, err
	}
	n := row.DecisionNote
	nodes, err := q.ListNoteNodes(ctx, id)
	if err != nil {
		return Note{}, err
	}
	tickets, err := q.ListNoteTickets(ctx, id)
	if err != nil {
		return Note{}, err
	}
	out := Note{
		Id: n.ID, Key: n.Key, ProjectKey: row.ProjectKey, Title: n.Title, DecidedOn: openapi_types.Date{Time: n.DecidedOn},
		Attendees: n.Attendees, Body: n.Body, Author: Ref{Id: n.CreatedBy, Name: row.AuthorName},
		CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt, Archived: n.ArchivedAt != nil, CanEdit: canEditNote(pc, n),
		Nodes: make([]NodeRef, len(nodes)), Tickets: []NoteTicket{},
	}
	if n.ClientID != nil {
		out.Client = &Ref{Id: *n.ClientID, Name: deref(row.ClientName)}
	}
	for i, nd := range nodes {
		out.Nodes[i] = NodeRef{Id: nd.ID, Name: nd.Name, Archived: nd.Archived}
	}
	if len(tickets) > 0 {
		ids := make([]int64, len(tickets))
		for i, t := range tickets {
			ids[i] = t.ID
		}
		seen, err := q.VisibleTicketIDs(ctx, db.VisibleTicketIDsParams{Ids: ids, IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID})
		if err != nil {
			return Note{}, err
		}
		for _, t := range tickets {
			if slices.Contains(seen, t.ID) {
				out.Tickets = append(out.Tickets, NoteTicket{Key: t.Key, Title: t.Title})
			}
		}
	}
	return out, nil
}

// noteAudit is what a note's history keeps, so every earlier version stays readable.
func noteAudit(n Note) map[string]any {
	menus := make([]string, len(n.Nodes))
	for i, nd := range n.Nodes {
		menus[i] = nd.Name
	}
	tickets := make([]string, len(n.Tickets))
	for i, t := range n.Tickets {
		tickets[i] = t.Key
	}
	m := map[string]any{
		"title": n.Title, "decided_on": n.DecidedOn.String(), "attendees": n.Attendees, "body": n.Body,
		"menus": menus, "tickets": tickets, "client": nil, "archived": n.Archived,
	}
	if n.Client != nil {
		m["client"] = n.Client.Name
	}
	return m
}

// indexNote queues a note's re-index in the caller's transaction (§13.2).
func (s *Server) indexNote(ctx context.Context, tx pgx.Tx, ids ...int64) error {
	if len(ids) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(ids))
	for i, id := range ids {
		params[i] = river.InsertManyParams{Args: indexer.IndexNote{NoteID: id}}
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, params)
	return err
}
