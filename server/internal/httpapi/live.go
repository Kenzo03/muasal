package httpapi

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

// ticketMerge is how long a stream gathers ticket changes before sending
// them together, so an import or a bulk change arrives as a few events.
const ticketMerge = 300 * time.Millisecond

// StreamEvents is a tab's one live stream (spec: live ticket updates): the
// caller's notifications and, with ?project=KEY, the IDs of that project's
// tickets that changed, merged and filtered to what the caller may see.
func (s *Server) StreamEvents(w http.ResponseWriter, r *http.Request, params StreamEventsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var projectID int64 // 0: no project; the stream still hears resyncs
	if params.Project != nil {
		pc, ok := s.projectFor(w, r, *params.Project, access.Viewer)
		if !ok {
			return
		}
		projectID = pc.project.ID
	}
	s.hub.start.Do(func() { go s.listen(s.bg) })
	notes, stopNotes := s.hub.subscribe(u.ID)
	defer stopNotes()
	changes, stopChanges := s.tickets.subscribe(projectID)
	defer stopChanges()
	send, stop := startSSE(w)
	defer stop()
	send("ready", map[string]any{})
	ctx := r.Context()
	pending := map[int64]bool{}
	var flush <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-notes:
			rows, err := s.q.ListNotifications(ctx, db.ListNotificationsParams{UserID: u.ID, IsAdmin: u.IsAdmin, ID: &id})
			if err == nil && len(rows) > 0 {
				send("notification", toAPINotification(rows[0]))
			}
		case id := <-changes:
			if id == 0 {
				send("resync", map[string]any{})
				continue
			}
			pending[id] = true
			if flush == nil {
				flush = time.After(ticketMerge)
			}
		case <-flush:
			flush = nil
			ids, member := s.visibleChanges(ctx, u, projectID, pending)
			clear(pending)
			if !member {
				return // removed from the project while watching
			}
			if len(ids) > 0 {
				send("tickets", map[string]any{"tickets": ids})
			}
		}
	}
}

// visibleChanges keeps the changed tickets u may see now: the scope is read
// fresh, so a narrowed or removed membership applies to the next batch.
// member is false once u is no longer in the project. A deleted ticket's ID
// passes, so an open page learns it is gone.
func (s *Server) visibleChanges(ctx context.Context, u *db.User, projectID int64, changed map[int64]bool) (ids []int64, member bool) {
	scope, member, err := access.ForProject(ctx, s.q, u, projectID)
	if err != nil {
		s.log.Warn("live updates: scope", "error", err)
		return nil, true
	}
	if !member {
		return nil, false
	}
	ids = slices.Sorted(maps.Keys(changed))
	rows, err := s.q.TicketClients(ctx, db.TicketClientsParams{ProjectID: projectID, Ids: ids})
	if err != nil {
		s.log.Warn("live updates: clients", "error", err)
		return nil, true
	}
	hidden := map[int64]bool{}
	for _, row := range rows {
		if !scope.Sees(row.ClientID) {
			hidden[row.ID] = true
		}
	}
	return slices.DeleteFunc(ids, func(id int64) bool { return hidden[id] }), true
}
