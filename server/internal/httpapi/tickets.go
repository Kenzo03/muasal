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
}

func (s *Server) CreateTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Member)
	if !ok {
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
		Priority: in.Priority, DueDate: in.DueDate,
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
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
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
		if out, err = readTicket(ctx, q, created.Key); err != nil {
			return err
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
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) GetTicket(w http.ResponseWriter, r *http.Request, key string) {
	_, row, ok := s.ticketFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	out, err := ticketFromRow(r.Context(), s.q, row)
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
	version, err := strconv.ParseInt(strings.Trim(params.IfMatch, `"`), 10, 32)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "If-Match must carry the ticket's version, as its ETag gave it")
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
		Priority: in.Priority, DueDate: in.DueDate,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.RequesterContactId == nil && in.RequesterUserId == nil {
		fields = append(fields, FieldError{Field: "requester_contact_id", Code: "required", Message: "Say who asked for this"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		before, err := ticketFromRow(ctx, q, row)
		if err != nil {
			return err
		}
		updated, err := q.UpdateTicket(ctx, db.UpdateTicketParams{
			ID: row.Ticket.ID, Version: int32(version), Type: draft.Type, Title: draft.Title, Description: draft.Description,
			Reason: draft.Reason, ClientID: draft.ClientID, RequesterContactID: draft.RequesterContactID,
			RequesterUserID: draft.RequesterUserID, AssigneeID: draft.AssigneeID, Priority: draft.Priority, DueDate: draft.DueDate,
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
		if out, err = readTicket(ctx, q, updated.Key); err != nil {
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

// TransitionTicket moves a ticket among open statuses (R-TK-3). Entering Done
// or Cancelled is a close, which needs the decision record of FSD §9.1.
func (s *Server) TransitionTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
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
	if closedCategory(st.Category) {
		writeProblem(w, http.StatusUnprocessableEntity, "close_unavailable", "Closing a ticket needs its decision record, which is not available yet")
		return
	}
	var out Ticket
	err = s.inTx(ctx, func(q *db.Queries) error {
		if st.ID != row.Ticket.StatusID {
			if _, err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: row.Ticket.ID, StatusID: st.ID}); err != nil {
				return err
			}
			if err := audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "transition",
				map[string]any{"status": map[string]any{"old": row.Status.Name, "new": st.Name}}); err != nil {
				return err
			}
		}
		var err error
		out, err = readTicket(ctx, q, row.Ticket.Key)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("ETag", etag(out.Version))
	writeJSON(w, http.StatusOK, out)
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
	nodeIDs := slices.Compact(slices.Sorted(slices.Values(in.NodeIDs)))
	if len(nodeIDs) > 0 {
		visible, err := s.q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
		if err != nil {
			return t, nil, nil, err
		}
		live := map[int64]bool{}
		for _, n := range visible {
			live[n.ID] = true
		}
		for _, id := range nodeIDs {
			if !live[id] {
				f = append(f, FieldError{Field: "node_ids", Code: "invalid", Message: "Choose menus and modules of this project"})
				break
			}
		}
	}
	return t, orEmpty(nodeIDs), f, nil
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
func readTicket(ctx context.Context, q *db.Queries, key string) (Ticket, error) {
	row, err := q.GetTicketByKey(ctx, key)
	if err != nil {
		return Ticket{}, err
	}
	return ticketFromRow(ctx, q, row)
}

func ticketFromRow(ctx context.Context, q *db.Queries, row db.GetTicketByKeyRow) (Ticket, error) {
	t := row.Ticket
	nodes, err := q.ListTicketNodes(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	files, err := q.ListAttachments(ctx, t.ID)
	if err != nil {
		return Ticket{}, err
	}
	out := Ticket{
		Id: t.ID, Key: t.Key, ProjectKey: row.ProjectKey, Title: t.Title, Type: TicketType(t.Type),
		Description: t.Description, Reason: t.Reason, Priority: Priority(t.Priority), Version: t.Version,
		Status: toAPIStatus(row.Status), Reporter: Ref{Id: t.ReporterID, Name: row.ReporterName},
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		Nodes: make([]NodeRef, len(nodes)), Attachments: make([]Attachment, len(files)),
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
		"client": nil, "assignee": nil, "due_date": nil,
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
