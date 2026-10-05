package httpapi

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

var errMergeTarget = errors.New("merge target is the node or below it")

// MergeNode folds a duplicate node into another (R-MR-6), as imports leave
// behind: its tickets, notes and live sub-nodes move to the target, its name
// and aliases join the target's aliases so questions naming it still find the
// target, and it is archived, keeping its history.
func (s *Server) MergeNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, from, ok := s.nodeFor(w, r, id, access.Admin)
	if !ok {
		return
	}
	var in MergeNodeJSONBody
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	bad := func() {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "into_id", Code: "invalid", Message: "Choose another live item of this project, not one below this one"})
	}
	into, err := s.q.GetNode(ctx, in.IntoId)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (into.ProjectID != from.ProjectID || into.ArchivedAt != nil || from.ArchivedAt != nil)) {
		bad()
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var out Node
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.LockProject(ctx, from.ProjectID); err != nil {
			return err
		}
		below, err := q.IsSelfOrDescendant(ctx, db.IsSelfOrDescendantParams{NodeID: from.ID, CandidateID: into.ID})
		if err != nil {
			return err
		}
		if below {
			return errMergeTarget
		}
		// Tickets and notes on the duplicate and everything below it name its path: re-index them.
		tickets, err := q.ListTicketIDsUnderNode(ctx, from.ID)
		if err != nil {
			return err
		}
		notes, err := q.ListNoteIDsUnderNode(ctx, from.ID)
		if err != nil {
			return err
		}
		if err := q.MoveNodeLinks(ctx, db.MoveNodeLinksParams{FromID: from.ID, IntoID: into.ID}); err != nil {
			return err
		}
		kids, err := q.ListChildIDs(ctx, &from.ID)
		if err != nil {
			return err
		}
		if len(kids) > 0 {
			siblings, err := q.ListSiblingIDs(ctx, db.ListSiblingIDsParams{ProjectID: into.ProjectID, ParentID: &into.ID, ExcludeID: 0})
			if err != nil {
				return err
			}
			if err := q.PlaceNodes(ctx, db.PlaceNodesParams{ParentID: &into.ID, Ids: append(siblings, kids...)}); err != nil {
				return err
			}
		}
		aliases := slices.Clone(into.Aliases)
		for _, a := range append([]string{from.Name}, from.Aliases...) {
			if !slices.ContainsFunc(aliases, func(x string) bool { return strings.EqualFold(x, a) }) && !strings.EqualFold(a, into.Name) {
				aliases = append(aliases, a)
			}
		}
		updated, err := q.UpdateNode(ctx, db.UpdateNodeParams{ID: into.ID, Aliases: aliases})
		if err != nil {
			return err
		}
		if _, err := q.UpdateNode(ctx, db.UpdateNodeParams{ID: from.ID, Archived: ptr(true)}); err != nil {
			return err
		}
		clients, err := q.ListNodeClients(ctx, into.ID)
		if err != nil {
			return err
		}
		out = toAPINode(updated, clients)
		meta := webMeta(r).inProject(from.ProjectID)
		if err := audit(ctx, q, meta, &pc.user.ID, "node", from.ID, "merge", map[string]any{"into": into.Name, "into_id": into.ID}); err != nil {
			return err
		}
		if err := audit(ctx, q, meta, &pc.user.ID, "node", into.ID, "merged_in", map[string]any{"from": from.Name, "from_id": from.ID}); err != nil {
			return err
		}
		return s.reindexNodes(ctx, tx, tickets, notes)
	})
	if errors.Is(err, errMergeTarget) {
		bad()
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

func (s *Server) reindexNodes(ctx context.Context, tx pgx.Tx, tickets, notes []int64) error {
	if err := s.index(ctx, tx, tickets...); err != nil {
		return err
	}
	return s.indexNote(ctx, tx, notes...)
}
