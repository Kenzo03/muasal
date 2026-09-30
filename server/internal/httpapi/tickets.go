package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var errStale = errors.New("the ticket changed since it was read")

var clientIDField = FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client of this project in your scope"}

var nodeIDsField = FieldError{Field: "node_ids", Code: "invalid", Message: "Choose menus and modules of this project"}

// ticketInput is a create or an update request, whichever arrived.
type ticketInput struct {
	Type        TicketType
	Title       string
	ClientID    *int64
	ContactID   *int64
	UserID      *int64
	NodeIDs     []int64
	Reason      *string
	Description *string
	AssigneeID  *int64
	Priority    *Priority
	DueDate     *openapi_types.Date
	Estimate    *float64 // hours (MSL-54)
}

func (s *Server) CreateTicket(w http.ResponseWriter, r *http.Request, key string, params CreateTicketParams) {
	pc, ok := s.projectFor(w, r, key, access.Member)
	if !ok {
		return
	}
	idem := strings.TrimSpace(deref(params.IdempotencyKey))
	if utf8.RuneCountInString(idem) > 200 {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "Idempotency-Key takes at most 200 characters")
		return
	}
	var in TicketCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	draft, nodeIDs, fields, err := s.checkTicket(ctx, pc, ticketInput{
		Type: in.Type, Title: in.Title, ClientID: in.ClientId, ContactID: in.RequesterContactId, UserID: in.RequesterUserId,
		NodeIDs: in.NodeIds, Reason: in.Reason, Description: in.Description, AssigneeID: in.AssigneeId,
		Priority: in.Priority, DueDate: in.DueDate, Estimate: in.EstimateHours,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	status, statusOK, err := s.startStatus(ctx, pc.project.ID, in.StatusId)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !statusOK {
		fields = append(fields, FieldError{Field: "status_id", Code: "invalid", Message: "Choose an open status of this project"})
	}
	var noteID int64 // MSL-11: filed from a note's action item
	if k := strings.ToUpper(strings.TrimSpace(deref(in.NoteKey))); k != "" {
		note, err := s.q.GetNoteByKey(ctx, k)
		if err == nil && note.DecisionNote.ProjectID == pc.project.ID && note.DecisionNote.ArchivedAt == nil && pc.scope.Sees(note.DecisionNote.ClientID) {
			noteID = note.DecisionNote.ID
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.fail(w, r, err)
			return
		} else {
			fields = append(fields, FieldError{Field: "note_key", Code: "invalid", Message: "Choose a decision note of this project"})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	replayed := false
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		// A retry with the same Idempotency-Key waits for the first request, then gets its ticket (§17.1).
		if idem != "" {
			if err := q.LockIdempotencyKey(ctx, db.LockIdempotencyKeyParams{UserID: pc.user.ID, Key: idem}); err != nil {
				return err
			}
			prev, err := q.GetIdempotentTicket(ctx, db.GetIdempotentTicketParams{UserID: pc.user.ID, Key: idem})
			if err == nil {
				replayed = true
				out, err = readTicket(ctx, q, prev, pc.user)
				return err
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		n, err := q.NextTicketNumber(ctx, pc.project.ID)
		if err != nil {
			return err
		}
		draft.Number, draft.Key, draft.StatusID = n, fmt.Sprintf("%s-%d", pc.project.Key, n), status
		created, err := q.CreateTicket(ctx, draft)
		if err != nil {
			return err
		}
		if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: created.ID, NodeIds: nodeIDs}); err != nil {
			return err
		}
		if created.AssigneeID != nil {
			if err := notify(ctx, q, "assigned", created.ID, &pc.user.ID, []int64{*created.AssigneeID}, nil); err != nil {
				return err
			}
		}
		if noteID != 0 {
			if err := q.LinkNoteTicket(ctx, db.LinkNoteTicketParams{NoteID: noteID, TicketID: created.ID}); err != nil {
				return err
			}
			if err := s.indexNote(ctx, tx, noteID); err != nil { // the note's chunk names its tickets
				return err
			}
		}
		if out, err = readTicket(ctx, q, created.Key, pc.user); err != nil {
			return err
		}
		if err := s.index(ctx, tx, created.ID); err != nil {
			return err
		}
		if idem != "" {
			if err := q.SaveIdempotencyKey(ctx, db.SaveIdempotencyKeyParams{UserID: pc.user.ID, Key: idem, TicketID: created.ID}); err != nil {
				return err
			}
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", created.ID, "create", ticketAudit(out))
	})
	if constraintOf(err) == "tickets_client_linked" {
		ticketClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
		writeJSON(w, http.StatusOK, out)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) GetTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	out, err := ticketFromRow(r.Context(), s.q, row, pc.user)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	writeJSON(w, http.StatusOK, out)
}

// UpdateTicket replaces a ticket's editable fields. If-Match carries the
// version the editor read; a stale one answers 412 (FSD §8.6, AC-TK-5).
func (s *Server) UpdateTicket(w http.ResponseWriter, r *http.Request, key string, params UpdateTicketParams) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	version, ok := ifMatch(w, params.IfMatch)
	if !ok {
		return
	}
	var in TicketUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	draft, nodeIDs, fields, err := s.checkTicket(ctx, pc, ticketInput{
		Type: in.Type, Title: in.Title, ClientID: in.ClientId, ContactID: in.RequesterContactId, UserID: in.RequesterUserId,
		NodeIDs: in.NodeIds, Reason: in.Reason, Description: in.Description, AssigneeID: in.AssigneeId,
		Priority: in.Priority, DueDate: in.DueDate, Estimate: in.EstimateHours,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.RequesterContactId == nil && in.RequesterUserId == nil {
		fields = append(fields, FieldError{Field: "requester_contact_id", Code: "required", Message: "Say who asked for this"})
	}
	// TK-3 holds after the close: a closed ticket keeps a reason and a menu (FSD §9.1).
	if closedCategory(row.Status.Category) {
		fields = append(fields, closeFieldErrors(draft.Reason, len(nodeIDs))...)
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		before, err := ticketFromRow(ctx, q, row, pc.user)
		if err != nil {
			return err
		}
		updated, err := q.UpdateTicket(ctx, db.UpdateTicketParams{
			ID: row.Ticket.ID, Version: version, Type: draft.Type, Title: draft.Title, Description: draft.Description,
			Reason: draft.Reason, ClientID: draft.ClientID, RequesterContactID: draft.RequesterContactID,
			RequesterUserID: draft.RequesterUserID, AssigneeID: draft.AssigneeID, Priority: draft.Priority, DueDate: draft.DueDate,
			EstimateHours: draft.EstimateHours,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStale
		}
		if err != nil {
			return err
		}
		if err := q.ClearTicketNodes(ctx, updated.ID); err != nil {
			return err
		}
		if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: updated.ID, NodeIds: nodeIDs}); err != nil {
			return err
		}
		if a := updated.AssigneeID; a != nil && (row.Ticket.AssigneeID == nil || *row.Ticket.AssigneeID != *a) {
			if err := notify(ctx, q, "assigned", updated.ID, &pc.user.ID, []int64{*a}, nil); err != nil {
				return err
			}
		}
		if out, err = readTicket(ctx, q, updated.Key, pc.user); err != nil {
			return err
		}
		if err := s.index(ctx, tx, updated.ID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", updated.ID, "update",
			changed(ticketAudit(before), ticketAudit(out)))
	})
	switch {
	case errors.Is(err, errStale):
		writeProblem(w, http.StatusPreconditionFailed, "stale", "Someone updated this ticket a moment ago")
	case constraintOf(err) == "tickets_client_linked":
		ticketClientInvalid(w)
	case err != nil:
		s.fail(w, r, err)
	default:
		w.Header().Set("ETag", etag(out.Version))
		writeJSON(w, http.StatusOK, out)
	}
}

// TransitionTicket moves a ticket to another status (R-TK-3). Entering Done or
// Cancelled is a close: the reason and menus it names and a confirmed decision
// record commit with the status in one transaction, or nothing changes (R-DC-1,
// R-DC-7). Leaving them reopens the ticket and turns its record back into a
// draft (R-DC-6). If-Match is optional; a stale version answers 412.
func (s *Server) TransitionTicket(w http.ResponseWriter, r *http.Request, key string, params TransitionTicketParams) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var version *int32
	if params.IfMatch != nil {
		v, ok := ifMatch(w, *params.IfMatch)
		if !ok {
			return
		}
		version = &v
	}
	var in TransitionRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	st, err := s.q.GetStatus(ctx, in.StatusId)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && st.ProjectID != row.Ticket.ProjectID) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "status_id", Code: "invalid", Message: "Choose a status of this project"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	moving, closing := st.ID != row.Ticket.StatusID, closedCategory(st.Category)
	reason := row.Ticket.Reason
	if in.Reason != nil {
		reason = strings.TrimSpace(*in.Reason)
	}
	var nodeIDs []int64 // replaces the ticket's menus when not nil
	var text decisionText
	if moving && closing {
		menus, err := s.q.ListTicketNodes(ctx, row.Ticket.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var fields []FieldError
		count := len(menus)
		if in.NodeIds != nil {
			ids, live, err := s.liveNodeIDs(ctx, pc, *in.NodeIds)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			if !live {
				fields = append(fields, nodeIDsField)
			}
			nodeIDs, count = ids, len(ids)
		}
		fields = append(fields, closeFieldErrors(reason, count)...)
		var decisionFields []FieldError
		text, decisionFields = checkDecision("decision.", in.Decision)
		if fields = append(fields, decisionFields...); len(fields) > 0 {
			writeProblem(w, http.StatusUnprocessableEntity, "close_validation_failed", "Ticket can't be closed yet", fields...)
			return
		}
	}
	var out Ticket
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		before, err := ticketFromRow(ctx, q, row, pc.user)
		if err != nil || !moving {
			out = before
			return err
		}
		_, err = q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.Ticket.ID, StatusID: st.ID, Closed: closing, Version: version})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStale
		}
		if err != nil {
			return err
		}
		action := ""
		switch {
		case closing:
			if in.Reason != nil {
				if err := q.SetTicketReason(ctx, db.SetTicketReasonParams{ID: row.Ticket.ID, Reason: reason}); err != nil {
					return err
				}
			}
			if nodeIDs != nil {
				if err := q.ClearTicketNodes(ctx, row.Ticket.ID); err != nil {
					return err
				}
				if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: row.Ticket.ID, NodeIds: nodeIDs}); err != nil {
					return err
				}
			}
			outcome := DecisionOutcomeImplemented // R-DC-2: Done implements, Cancelled rejects
			if st.Category == string(StatusCategoryCancelled) {
				outcome = DecisionOutcomeRejected
			}
			if _, err := q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
				TicketID: row.Ticket.ID, WhatChanged: text.WhatChanged, Why: text.Why, Alternatives: text.Alternatives,
				Outcome: string(outcome), ConfirmedBy: pc.user.ID, AiDrafted: text.AIDrafted,
			}); err != nil {
				return err
			}
			if err := q.RefreshSuperseded(ctx, row.Ticket.ID); err != nil { // a record written after its reverses link (R-TK-5)
				return err
			}
			action = "decision_confirm"
		case closedCategory(row.Status.Category):
			if err := q.DraftDecision(ctx, row.Ticket.ID); err != nil {
				return err
			}
			action = "decision_draft"
		}
		if out, err = readTicket(ctx, q, row.Ticket.Key, pc.user); err != nil {
			return err
		}
		if err := s.index(ctx, tx, row.Ticket.ID); err != nil {
			return err
		}
		m := webMeta(r).inProject(pc.project.ID)
		if err := audit(ctx, q, m, &pc.user.ID, "ticket", row.Ticket.ID, "transition", changed(ticketAudit(before), ticketAudit(out))); err != nil {
			return err
		}
		if err := notify(ctx, q, "status", row.Ticket.ID, &pc.user.ID, []int64{row.Ticket.ReporterID, deref(row.Ticket.AssigneeID)},
			map[string]any{"status": st.Name, "from": row.Status.Name}); err != nil {
			return err
		}
		if d := changed(decisionAudit(before.Decision), decisionAudit(out.Decision)); action != "" && len(d) > 0 {
			return audit(ctx, q, m, &pc.user.ID, "ticket", row.Ticket.ID, action, d)
		}
		return nil
	})
	switch {
	case errors.Is(err, errStale):
		writeProblem(w, http.StatusPreconditionFailed, "stale", "Someone updated this ticket a moment ago")
	case err != nil:
		s.fail(w, r, err)
	default:
		w.Header().Set("ETag", etag(out.Version))
		writeJSON(w, http.StatusOK, out)
	}
}

// ticketFor loads a ticket for the signed-in user. A ticket in a project the
// user does not belong to, or of a client outside their scope, answers 404 like
// a missing one (R-AC-3, R-AC-7); a role below need answers 403.
func (s *Server) ticketFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, db.GetTicketByKeyRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	ctx := r.Context()
	row, err := s.q.GetTicketByKey(ctx, strings.ToUpper(key))
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Ticket not found")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	p, err := s.q.GetProjectByID(ctx, row.Ticket.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if !pc.scope.Sees(row.Ticket.ClientID) {
		writeProblem(w, http.StatusNotFound, "not_found", "Ticket not found")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	if !pc.scope.Allows(need) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return projectCtx{}, db.GetTicketByKeyRow{}, false
	}
	return pc, row, true
}

// checkTicket validates a ticket's fields against the project and the caller's
// scope (FSD §8.1). It returns the row to store, minus number, key and status,
// and the distinct menu ids.
func (s *Server) checkTicket(ctx context.Context, pc projectCtx, in ticketInput) (db.CreateTicketParams, []int64, []FieldError, error) {
	var f []FieldError
	t := db.CreateTicketParams{
		ProjectID: pc.project.ID, Type: string(in.Type), Title: strings.TrimSpace(in.Title),
		Description: deref(in.Description), Reason: strings.TrimSpace(deref(in.Reason)),
		ClientID: in.ClientID, ReporterID: pc.user.ID, AssigneeID: in.AssigneeID, Priority: string(PriorityMedium),
	}
	if n := utf8.RuneCountInString(t.Title); n < 5 || n > 200 {
		f = append(f, FieldError{Field: "title", Code: "invalid", Message: "Use 5 to 200 characters"})
	}
	if !in.Type.Valid() {
		f = append(f, FieldError{Field: "type", Code: "invalid", Message: "Choose bug, change request or feature"})
	}
	if in.Priority != nil {
		if !in.Priority.Valid() {
			f = append(f, FieldError{Field: "priority", Code: "invalid", Message: "Choose low, medium, high or urgent"})
		}
		t.Priority = string(*in.Priority)
	}
	if utf8.RuneCountInString(t.Reason) > 2000 {
		f = append(f, FieldError{Field: "reason", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	if utf8.RuneCountInString(t.Description) > 50000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 50,000 characters"})
	}
	if in.DueDate != nil {
		t.DueDate = &in.DueDate.Time
	}
	if in.Estimate != nil {
		if *in.Estimate < 0 || *in.Estimate > 9999 {
			f = append(f, FieldError{Field: "estimate_hours", Code: "invalid", Message: "Use 0 to 9,999 hours"})
		}
		t.EstimateHours = in.Estimate
	}
	// The client is in the caller's scope; the database checks it is linked (AC-TK-4).
	if !pc.scope.Sees(in.ClientID) {
		f = append(f, clientIDField)
	}
	switch {
	case in.ContactID != nil && in.UserID != nil:
		f = append(f, FieldError{Field: "requester_contact_id", Code: "invalid", Message: "Choose a contact or a user, not both"})
	case in.ContactID != nil:
		rows, err := s.q.ListContacts(ctx, db.ListContactsParams{IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID, ID: in.ContactID})
		if err != nil {
			return t, nil, nil, err
		}
		if len(rows) == 0 || (rows[0].ClientID != nil && (in.ClientID == nil || *rows[0].ClientID != *in.ClientID)) {
			f = append(f, FieldError{Field: "requester_contact_id", Code: "invalid", Message: "Choose a contact of this client or an internal one"})
		}
		t.RequesterContactID = in.ContactID
	default:
		uid := pc.user.ID
		if in.UserID != nil && *in.UserID != uid {
			uid = *in.UserID
			_, member, err := access.ForProject(ctx, s.q, &db.User{ID: uid}, pc.project.ID)
			if err != nil {
				return t, nil, nil, err
			}
			if !member {
				f = append(f, FieldError{Field: "requester_user_id", Code: "invalid", Message: "Choose a member of this project"})
			}
		}
		t.RequesterUserID = &uid
	}
	if in.AssigneeID != nil {
		sc, member, err := access.ForProject(ctx, s.q, &db.User{ID: *in.AssigneeID}, pc.project.ID)
		if err != nil {
			return t, nil, nil, err
		}
		if !member || !sc.Allows(access.Member) || !sc.Sees(in.ClientID) {
			f = append(f, FieldError{Field: "assignee_id", Code: "invalid", Message: "Choose a member who can see this ticket"})
		}
	}
	nodeIDs, live, err := s.liveNodeIDs(ctx, pc, in.NodeIDs)
	if err != nil {
		return t, nil, nil, err
	}
	if !live {
		f = append(f, nodeIDsField)
	}
	return t, nodeIDs, f, nil
}

// liveNodeIDs sorts and dedupes ids, and reports whether each is a live node of
// the project that the caller sees.
func (s *Server) liveNodeIDs(ctx context.Context, pc projectCtx, ids []int64) ([]int64, bool, error) {
	out := slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(out) == 0 {
		return []int64{}, true, nil
	}
	visible, err := s.q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
	if err != nil {
		return nil, false, err
	}
	live := map[int64]bool{}
	for _, n := range visible {
		live[n.ID] = true
	}
	for _, id := range out {
		if !live[id] {
			return out, false, nil
		}
	}
	return out, true, nil
}

// startStatus picks a new ticket's status: the one asked for when it is an open
// status of the project, else the project's default.
func (s *Server) startStatus(ctx context.Context, projectID int64, id *int64) (int64, bool, error) {
	if id == nil {
		st, err := s.q.GetDefaultStatus(ctx, projectID)
		return st.ID, err == nil, err
	}
	st, err := s.q.GetStatus(ctx, *id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return st.ID, st.ProjectID == projectID && !closedCategory(st.Category), nil
}

// readTicket reads a ticket the way the API shows it.
func readTicket(ctx context.Context, q *db.Queries, key string, u *db.User) (Ticket, error) {
	row, err := q.GetTicketByKey(ctx, key)
	if err != nil {
		return Ticket{}, err
	}
	return ticketFromRow(ctx, q, row, u)
}

func ticketFromRow(ctx context.Context, q *db.Queries, row db.GetTicketByKeyRow, u *db.User) (Ticket, error) {
	t := row.Ticket
	nodes, err := q.ListTicketNodes(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	files, err := q.ListAttachments(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	decision, err := decisionOf(ctx, q, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	links, err := linksOf(ctx, q, t.ID, u)
	if err != nil {
		return Ticket{}, err
	}
	code, err := codeOf(ctx, q, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	out := Ticket{
		Id: t.ID, Key: t.Key, ProjectKey: row.ProjectKey, Title: t.Title, Type: TicketType(t.Type),
		Description: t.Description, Reason: t.Reason, Priority: Priority(t.Priority), Version: t.Version,
		Status: toAPIStatus(row.Status), Reporter: Ref{Id: t.ReporterID, Name: row.ReporterName},
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, ClosedAt: t.ClosedAt, Decision: decision, Links: links, Code: code,
		Nodes: make([]NodeRef, len(nodes)), Attachments: make([]Attachment, len(files)), EstimateHours: t.EstimateHours,
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(row.ClientName)}
	}
	if t.AssigneeID != nil {
		out.Assignee = &Ref{Id: *t.AssigneeID, Name: deref(row.AssigneeName)}
	}
	if t.RequesterContactID != nil {
		out.Requester = TicketRequester{Kind: TicketRequesterKindContact, Id: *t.RequesterContactID,
			Name: deref(row.RequesterContactName), Title: row.RequesterContactTitle}
	} else {
		out.Requester = TicketRequester{Kind: TicketRequesterKindUser, Id: *t.RequesterUserID, Name: deref(row.RequesterUserName)}
	}
	if t.DueDate != nil {
		out.DueDate = &openapi_types.Date{Time: *t.DueDate}
	}
	for i, n := range nodes {
		out.Nodes[i] = NodeRef{Id: n.ID, Name: n.Name, Archived: n.Archived}
	}
	for i, a := range files {
		out.Attachments[i] = toAPIAttachment(a.Attachment, a.UploaderName)
	}
	return out, nil
}

// ticketAudit is what a ticket's history shows: names, as they read at the time.
func ticketAudit(t Ticket) map[string]any {
	menus := make([]string, len(t.Nodes))
	for i, n := range t.Nodes {
		menus[i] = n.Name
	}
	m := map[string]any{
		"title": t.Title, "type": string(t.Type), "priority": string(t.Priority), "reason": t.Reason,
		"description": t.Description, "status": t.Status.Name, "requester": t.Requester.Name, "menus": menus,
		"client": nil, "assignee": nil, "due_date": nil, "estimate_hours": t.EstimateHours,
	}
	if t.Client != nil {
		m["client"] = t.Client.Name
	}
	if t.Assignee != nil {
		m["assignee"] = t.Assignee.Name
	}
	if t.DueDate != nil {
		m["due_date"] = t.DueDate.String()
	}
	return m
}

func toAPIAttachment(a db.Attachment, uploader string) Attachment {
	return Attachment{
		Id: a.ID, Filename: a.Filename, ContentType: a.ContentType, SizeBytes: a.SizeBytes,
		Uploader: Ref{Id: a.UploaderID, Name: uploader}, CreatedAt: a.CreatedAt,
	}
}

func ticketClientInvalid(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", clientIDField)
}

func etag(version int32) string { return `"` + strconv.Itoa(int(version)) + `"` }

// ifMatch reads the version an If-Match header carries; a malformed one answers 400.
func ifMatch(w http.ResponseWriter, header string) (int32, bool) {
	v, err := strconv.ParseInt(strings.Trim(header, `"`), 10, 32)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "If-Match must carry the ticket's version, as its ETag gave it")
		return 0, false
	}
	return int32(v), true
}
