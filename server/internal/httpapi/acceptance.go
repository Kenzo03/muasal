package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

// AcceptTicket records who at the client accepted a ticket's work, and when
// (MSL-66): a contact of the ticket's client, or any client's for core work,
// on today or an earlier day of the caller's calendar.
func (s *Server) AcceptTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in AcceptanceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	contacts, err := s.q.ListContacts(r.Context(), db.ListContactsParams{IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID, ID: &in.ContactId})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var f []FieldError
	client := row.Ticket.ClientID
	if len(contacts) == 0 || contacts[0].ClientID == nil || (client != nil && *contacts[0].ClientID != *client) {
		f = append(f, FieldError{Field: "contact_id", Code: "invalid", Message: "Choose a contact of this ticket's client"})
	}
	if in.AcceptedOn.After(s.today(pc.user)) {
		f = append(f, FieldError{Field: "accepted_on", Code: "invalid", Message: "Choose today or an earlier day"})
	}
	note := strings.TrimSpace(deref(in.Note))
	if utf8.RuneCountInString(note) > 2000 {
		f = append(f, FieldError{Field: "note", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	if len(f) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", f...)
		return
	}
	s.setAcceptance(w, r, pc, row, &in.ContactId, &in.AcceptedOn.Time, note,
		map[string]any{"accepted_by": contacts[0].Name, "accepted_on": in.AcceptedOn.String(), "note": note})
}

// UnacceptTicket removes an acceptance, such as one recorded by mistake.
func (s *Server) UnacceptTicket(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	s.setAcceptance(w, r, pc, row, nil, nil, "", map[string]any{"accepted_by": nil})
}

func (s *Server) setAcceptance(w http.ResponseWriter, r *http.Request, pc projectCtx, row db.GetTicketByKeyRow, contactID *int64, on *time.Time, note string, changes map[string]any) {
	ctx := r.Context()
	var out Ticket
	err := s.inTx(ctx, func(q *db.Queries) error {
		updated, err := q.SetTicketAcceptance(ctx, db.SetTicketAcceptanceParams{
			ContactID: contactID, AcceptedOn: on, Note: note, ID: row.Ticket.ID, Version: row.Ticket.Version,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errStale
		}
		if err != nil {
			return err
		}
		if out, err = readTicket(ctx, q, updated.Key, pc.user); err != nil {
			return err
		}
		action := "accept"
		if contactID == nil {
			action = "unaccept"
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", updated.ID, action, changes)
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
