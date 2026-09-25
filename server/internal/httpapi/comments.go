package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// GetTicketActivity interleaves a ticket's comments and history, oldest first
// (FSD §8.7). A deleted comment keeps its place; only system admins read it.
func (s *Server) GetTicketActivity(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	ctx := r.Context()
	comments, err := s.q.ListComments(ctx, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	events, err := s.q.ListTicketEvents(ctx, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]ActivityItem, 0, len(comments)+len(events))
	for _, c := range comments {
		items = append(items, commentItem(c.Comment, c.AuthorName, pc.user.IsAdmin))
	}
	for _, ev := range events {
		item := ActivityItem{Kind: ActivityItemKindEvent, At: ev.OccurredAt, Action: &ev.Action}
		if ev.ActorID != nil {
			item.Actor = &Ref{Id: *ev.ActorID, Name: deref(ev.ActorName)}
		}
		var changes map[string]any
		if err := json.Unmarshal(ev.Changes, &changes); err != nil {
			s.fail(w, r, err)
			return
		}
		item.Changes = &changes
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At.Before(items[j].At) })
	writeJSON(w, http.StatusOK, ActivityList{Items: items})
}

// CreateComment adds a comment, Internal unless marked client-safe (FSD §8.7,
// AC-TK-10). The comment row is its own record; edits and deletes are audited.
func (s *Server) CreateComment(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in CommentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	body, ok := commentBody(w, in.Body)
	if !ok {
		return
	}
	ctx := r.Context()
	var c db.Comment
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var err error
		c, err = q.CreateComment(ctx, db.CreateCommentParams{
			TicketID: row.Ticket.ID, AuthorID: pc.user.ID, Internal: in.Internal == nil || *in.Internal, Body: body,
		})
		if err != nil {
			return err
		}
		return s.index(ctx, tx, row.Ticket.ID)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, commentItem(c, pc.user.Name, true))
}

// UpdateComment rewords the author's comment; the history keeps the earlier
// text (AC-TK-6).
func (s *Server) UpdateComment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, c, ok := s.ownComment(w, r, id)
	if !ok {
		return
	}
	var in CommentUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	body, ok := commentBody(w, in.Body)
	if !ok {
		return
	}
	ctx := r.Context()
	var updated db.Comment
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		var err error
		if updated, err = q.UpdateCommentBody(ctx, db.UpdateCommentBodyParams{ID: id, Body: body}); err != nil {
			return err
		}
		if err := s.index(ctx, tx, c.TicketID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", c.TicketID, "comment_edit",
			map[string]any{"comment_id": id, "body": map[string]any{"old": c.Body, "new": body}})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Comment not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, commentItem(updated, pc.user.Name, true))
}

// DeleteComment hides the author's comment: readers see "Comment deleted" and
// only system admins still read the text (FSD §8.7).
func (s *Server) DeleteComment(w http.ResponseWriter, r *http.Request, id int64) {
	pc, c, ok := s.ownComment(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.DeleteComment(ctx, id); err != nil {
			return err
		}
		if err := s.index(ctx, tx, c.TicketID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", c.TicketID, "comment_delete",
			map[string]any{"comment_id": id})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ownComment loads a live comment for its author: 404 when the comment or its
// ticket is hidden or gone, 403 for anyone but the author.
func (s *Server) ownComment(w http.ResponseWriter, r *http.Request, id int64) (projectCtx, db.Comment, bool) {
	if s.requireUser(w, r) == nil {
		return projectCtx{}, db.Comment{}, false
	}
	row, err := s.q.GetComment(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Comment.DeletedAt != nil) {
		writeProblem(w, http.StatusNotFound, "not_found", "Comment not found")
		return projectCtx{}, db.Comment{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Comment{}, false
	}
	pc, _, ok := s.ticketFor(w, r, row.TicketKey, access.Member)
	if !ok {
		return projectCtx{}, db.Comment{}, false
	}
	if row.Comment.AuthorID != pc.user.ID {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only the author can change a comment")
		return projectCtx{}, db.Comment{}, false
	}
	return pc, row.Comment, true
}

func commentBody(w http.ResponseWriter, body string) (string, bool) {
	b := strings.TrimSpace(body)
	if b == "" || utf8.RuneCountInString(b) > 20000 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "body", Code: "required", Message: "Write a comment of at most 20,000 characters"})
		return "", false
	}
	return b, true
}

// commentItem shows a comment; a deleted one keeps its text only when
// showDeleted (system admins, or the author's own response).
func commentItem(c db.Comment, author string, showDeleted bool) ActivityItem {
	item := ActivityItem{
		Kind: ActivityItemKindComment, At: c.CreatedAt, Actor: &Ref{Id: c.AuthorID, Name: author},
		CommentId: &c.ID, Internal: &c.Internal, Deleted: ptr(c.DeletedAt != nil), Edited: ptr(c.EditedAt != nil),
	}
	if c.DeletedAt == nil || showDeleted {
		item.Body = &c.Body
	}
	return item
}
