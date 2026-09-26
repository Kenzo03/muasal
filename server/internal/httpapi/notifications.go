package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
)

// hub fans notifications out to each user's open streams. One LISTEN
// connection feeds it (FSD §8.10); it starts with the first stream.
type hub struct {
	mu    sync.Mutex
	subs  map[int64]map[chan int64]struct{}
	start sync.Once
}

func (h *hub) subscribe(userID int64) (chan int64, func()) {
	ch := make(chan int64, 16)
	h.mu.Lock()
	if h.subs == nil {
		h.subs = map[int64]map[chan int64]struct{}{}
	}
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan int64]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs[userID], ch)
		h.mu.Unlock()
	}
}

func (h *hub) publish(userID, id int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[userID] {
		select {
		case ch <- id:
		default: // a stuck tab misses a push; its bell still counts on the next load
		}
	}
}

// listen holds one connection on LISTEN muasal_notifications and publishes
// each "user:id" payload, reconnecting after a failure.
func (s *Server) listen(ctx context.Context) {
	for ctx.Err() == nil {
		err := func() error {
			conn, err := s.pool.Acquire(ctx)
			if err != nil {
				return err
			}
			defer conn.Release()
			if _, err := conn.Exec(ctx, "LISTEN muasal_notifications"); err != nil {
				return err
			}
			for {
				n, err := conn.Conn().WaitForNotification(ctx)
				if err != nil {
					return err
				}
				user, id, ok := strings.Cut(n.Payload, ":")
				uid, err1 := strconv.ParseInt(user, 10, 64)
				nid, err2 := strconv.ParseInt(id, 10, 64)
				if ok && err1 == nil && err2 == nil {
					s.hub.publish(uid, nid)
				}
			}
		}()
		if err != nil && ctx.Err() == nil {
			s.log.Warn("notification listener", "error", err)
			time.Sleep(time.Second)
		}
	}
}

// ListNotifications returns the caller's latest 50 and the unread count.
func (s *Server) ListNotifications(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rows, err := s.q.ListNotifications(ctx, db.ListNotificationsParams{UserID: u.ID, IsAdmin: u.IsAdmin})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	unread, err := s.q.CountUnread(ctx, u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := NotificationList{Items: make([]Notification, len(rows)), Unread: int(unread)}
	for i, n := range rows {
		out.Items[i] = toAPINotification(n)
	}
	writeJSON(w, http.StatusOK, out)
}

// MarkNotificationsRead marks one notification read, or all.
func (s *Server) MarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in MarkNotificationsReadJSONBody
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := s.q.MarkRead(r.Context(), db.MarkReadParams{UserID: u.ID, ID: in.Id}); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// StreamNotifications pushes each new notification to the caller's open tab
// as it is recorded (§8.10). Access is checked again as each one goes out.
func (s *Server) StreamNotifications(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	s.hub.start.Do(func() { go s.listen(s.bg) })
	ch, cancel := s.hub.subscribe(u.ID)
	defer cancel()
	send, stop := startSSE(w)
	defer stop()
	send("ready", map[string]any{})
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-ch:
			rows, err := s.q.ListNotifications(ctx, db.ListNotificationsParams{UserID: u.ID, IsAdmin: u.IsAdmin, ID: &id})
			if err != nil || len(rows) == 0 {
				continue
			}
			send("notification", toAPINotification(rows[0]))
		}
	}
}

// ListMentionable lists who an @mention on the ticket can reach.
func (s *Server) ListMentionable(w http.ResponseWriter, r *http.Request, key string) {
	_, row, ok := s.ticketFor(w, r, key, "viewer")
	if !ok {
		return
	}
	rows, err := s.q.ListMentionable(r.Context(), row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := MentionableList{Items: make([]Mentionable, len(rows))}
	for i, m := range rows {
		out.Items[i] = Mentionable{Id: m.ID, Name: m.Name, Handle: m.Handle}
	}
	writeJSON(w, http.StatusOK, out)
}

func toAPINotification(n db.ListNotificationsRow) Notification {
	out := Notification{Id: n.ID, Type: NotificationType(n.Type), CreatedAt: n.CreatedAt, Read: n.ReadAt != nil,
		TicketKey: n.TicketKey, TicketTitle: n.TicketTitle, Payload: map[string]any{}}
	_ = json.Unmarshal(n.Payload, &out.Payload)
	if n.ActorID != nil {
		out.Actor = &Ref{Id: *n.ActorID, Name: deref(n.ActorName)}
	}
	return out
}

// notify records a ticket event for these users, except the actor, those who
// cannot see the ticket and those who turned the event off (§8.10).
func notify(ctx context.Context, q *db.Queries, typ string, ticketID int64, actor *int64, users []int64, payload map[string]any) error {
	users = slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(users))), func(id int64) bool { return id == 0 })
	if len(users) == 0 {
		return nil
	}
	b, _ := json.Marshal(payload)
	_, err := q.NotifyTicket(ctx, db.NotifyTicketParams{Type: typ, TicketID: ticketID, ActorID: actor, UserIds: users, Payload: b})
	return err
}

// mentionRe finds @handles: the part of a member's email before the @.
var mentionRe = regexp.MustCompile(`(?:^|[^\w@.])@([a-zA-Z0-9][a-zA-Z0-9._-]{0,63})`)

// notifyComment tells the ticket's reporter, assignee and earlier commenters
// about a new comment, and the members it @mentions (§8.10).
func notifyComment(ctx context.Context, q *db.Queries, t db.Ticket, author int64, body string) error {
	excerpt := strings.Join(strings.Fields(body), " ")
	if r := []rune(excerpt); len(r) > 140 {
		excerpt = string(r[:140]) + "…"
	}
	payload := map[string]any{"excerpt": excerpt}
	var mentioned []int64
	if handles := mentionRe.FindAllStringSubmatch(body, -1); len(handles) > 0 {
		members, err := q.ListMentionable(ctx, t.ID)
		if err != nil {
			return err
		}
		for _, h := range handles {
			for _, m := range members {
				if strings.EqualFold(m.Handle, strings.TrimRight(h[1], ".")) {
					mentioned = append(mentioned, m.ID)
				}
			}
		}
		if err := notify(ctx, q, "mention", t.ID, &author, mentioned, payload); err != nil {
			return err
		}
	}
	commenters, err := q.ListCommenters(ctx, t.ID)
	if err != nil {
		return err
	}
	users := append([]int64{t.ReporterID, deref(t.AssigneeID)}, commenters...)
	users = slices.DeleteFunc(users, func(id int64) bool { return slices.Contains(mentioned, id) }) // a mention says it already
	return notify(ctx, q, "comment", t.ID, &author, users, payload)
}

// notifyPrefs reads a user's stored preferences.
func notifyPrefs(u db.User) *NotifyPrefs {
	var p NotifyPrefs
	if err := json.Unmarshal(u.NotifyPrefs, &p); err != nil {
		return &NotifyPrefs{}
	}
	return &p
}
