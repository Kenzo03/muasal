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

// linksOf reads a ticket's links in both directions, keeping those whose other
// ticket u may see (R-TK-6).
func linksOf(ctx context.Context, q *db.Queries, ticketID int64, u *db.User) ([]TicketLink, error) {
	rows, err := q.ListTicketLinks(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.OtherID
	}
	visible := ids
	if len(ids) > 0 {
		if visible, err = q.VisibleTicketIDs(ctx, db.VisibleTicketIDsParams{Ids: ids, IsAdmin: u.IsAdmin, UserID: u.ID}); err != nil {
			return nil, err
		}
	}
	out := []TicketLink{}
	for _, r := range rows {
		if !slices.Contains(visible, r.OtherID) {
			continue
		}
		out = append(out, TicketLink{Id: r.ID, Type: LinkType(r.Type), Outgoing: r.Outgoing, Ticket: LinkedTicket{
			Key: r.OtherKey, Title: r.OtherTitle, Status: toAPIStatus(r.Status), ClosedAt: r.OtherClosedAt,
		}})
	}
	return out, nil
}

// CreateLink links this ticket to another the member may see, in any project
// (FSD §8.8). A reverses link supersedes the other ticket's decision (R-TK-5).
func (s *Server) CreateLink(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in LinkCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	if !slices.Contains([]LinkType{LinkTypeReverses, LinkTypeExtends, LinkTypeRelatedTo}, in.Type) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "type", Code: "invalid", Message: "Choose reverses, extends or related to"})
		return
	}
	ctx := r.Context()
	otherKey := strings.ToUpper(strings.TrimSpace(in.Key))
	notFound := FieldError{Field: "key", Code: "not_found", Message: "No ticket with this key that you can see"}
	other, err := s.q.GetTicketByKey(ctx, otherKey)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", notFound)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	seen, err := s.q.VisibleTicketIDs(ctx, db.VisibleTicketIDsParams{Ids: []int64{other.Ticket.ID}, IsAdmin: pc.user.IsAdmin, UserID: pc.user.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(seen) == 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", notFound)
		return
	}
	if other.Ticket.ID == row.Ticket.ID {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "key", Code: "self_link", Message: "A ticket cannot link to itself"})
		return
	}
	if in.Type == LinkTypeReverses { // it supersedes the other ticket's decision
		scope, _, err := access.ForProject(ctx, s.q, pc.user, other.Ticket.ProjectID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !scope.Allows(access.Member) {
			writeProblem(w, http.StatusForbidden, "forbidden", "Your role in the other ticket's project does not allow this")
			return
		}
	}
	var out TicketLink
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		l, err := q.CreateLink(ctx, db.CreateLinkParams{FromID: row.Ticket.ID, ToID: other.Ticket.ID, Type: string(in.Type), CreatedBy: pc.user.ID})
		if err != nil {
			return err // ErrNoRows: the same link exists
		}
		if in.Type == LinkTypeReverses {
			if err := q.RefreshSuperseded(ctx, other.Ticket.ID); err != nil {
				return err
			}
		}
		if err := s.auditLink(ctx, q, r, pc, "link", l, row.Ticket, other.Ticket); err != nil {
			return err
		}
		out = TicketLink{Id: l.ID, Type: in.Type, Outgoing: true, Ticket: LinkedTicket{
			Key: other.Ticket.Key, Title: other.Ticket.Title, Status: toAPIStatus(other.Status), ClosedAt: other.Ticket.ClosedAt,
		}}
		return s.index(ctx, tx, row.Ticket.ID, other.Ticket.ID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusConflict, "link_exists", "These tickets already have this link")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// DeleteLink removes a link. Members of the linking ticket's project may, and
// for a reverses link members of the other ticket's project too; the other
// ticket's decision is current again unless another reverses link still points
// at it (R-TK-7).
func (s *Server) DeleteLink(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	l, err := s.q.GetLink(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Link not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from, err := s.q.GetTicketByID(ctx, l.FromID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pc, row, ok := s.ticketFor(w, r, from.Key, access.Member)
	if !ok {
		return
	}
	to, err := s.q.GetTicketByID(ctx, l.ToID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if l.Type == string(LinkTypeReverses) { // the other decision becomes current again
		scope, _, err := access.ForProject(ctx, s.q, pc.user, to.ProjectID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !scope.Allows(access.Member) {
			writeProblem(w, http.StatusForbidden, "forbidden", "Your role in the other ticket's project does not allow this")
			return
		}
	}
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.DeleteLink(ctx, l.ID); err != nil {
			return err
		}
		if l.Type == string(LinkTypeReverses) {
			if err := q.RefreshSuperseded(ctx, l.ToID); err != nil {
				return err
			}
		}
		if err := s.auditLink(ctx, q, r, pc, "unlink", l, row.Ticket, to); err != nil {
			return err
		}
		return s.index(ctx, tx, l.FromID, l.ToID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// auditLink records a link change in both tickets' history, each in its own
// project, with the wording that ticket shows.
func (s *Server) auditLink(ctx context.Context, q *db.Queries, r *http.Request, pc projectCtx, action string, l db.TicketLink, from, to db.Ticket) error {
	if err := audit(ctx, q, webMeta(r).inProject(from.ProjectID), &pc.user.ID, "ticket", from.ID, action,
		map[string]any{"type": l.Type, "outgoing": true, "key": to.Key}); err != nil {
		return err
	}
	return audit(ctx, q, webMeta(r).inProject(to.ProjectID), &pc.user.ID, "ticket", to.ID, action,
		map[string]any{"type": l.Type, "outgoing": false, "key": from.Key})
}
