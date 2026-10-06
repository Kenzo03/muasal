package httpapi_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// listenTickets opens a LISTEN on zettra_tickets and returns a function that
// collects the payloads arriving within d.
func listenTickets(t *testing.T, pool *pgxpool.Pool) func(d time.Duration) []string {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Release)
	if _, err := conn.Exec(context.Background(), "LISTEN zettra_tickets"); err != nil {
		t.Fatal(err)
	}
	return func(d time.Duration) []string {
		var got []string
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		for {
			n, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return got
			}
			got = append(got, n.Payload)
		}
	}
}

// Every write to what a ticket page shows announces project:ticket on commit;
// a rolled-back write announces nothing (spec: live ticket updates).
func TestTicketChangesSignal(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	a := e.seedTicket(w.p, w.pmUser, "Overtime rounding", &w.a, w.ot)
	b := e.seedTicket(w.p, w.pmUser, "Overtime cap", &w.a, w.ot)
	collect := listenTickets(t, e.d.Pool)
	want := func(ids ...int64) []string {
		var out []string
		for _, id := range ids {
			out = append(out, fmt.Sprintf("%d:%d", w.p.ID, id))
		}
		return out
	}
	ctx := context.Background()

	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET title = 'Overtime rounding v2' WHERE id = $1", a.ID); err != nil {
		t.Fatal(err)
	}
	if got := collect(time.Second); !slices.Equal(got, want(a.ID)) {
		t.Fatalf("ticket update: %v", got)
	}

	if code := e.call(w.pm, http.MethodPost, "/tickets/"+a.Key+"/comments", map[string]any{"body": "Checked with Budi."}, nil); code != http.StatusCreated {
		t.Fatalf("comment: %d", code)
	}
	if got := collect(time.Second); !slices.Contains(got, want(a.ID)[0]) {
		t.Fatalf("comment: %v", got)
	}

	if _, err := e.d.Pool.Exec(ctx, "INSERT INTO ticket_links (from_id, to_id, type, created_by) VALUES ($1, $2, 'related_to', $3)", a.ID, b.ID, w.pmUser.ID); err != nil {
		t.Fatal(err)
	}
	got := collect(time.Second)
	slices.Sort(got)
	if !slices.Equal(got, want(min(a.ID, b.ID), max(a.ID, b.ID))) {
		t.Fatalf("link: %v", got)
	}

	tx, err := e.d.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE tickets SET title = 'Never saved' WHERE id = $1", a.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := collect(500 * time.Millisecond); len(got) != 0 {
		t.Fatalf("rolled back, still signalled: %v", got)
	}
}

type sse struct{ event, data string }

// openEvents opens /events for the client and returns its events; it waits
// for `ready` and for the listener's LISTEN to be in place.
func openEvents(t *testing.T, e *env, c *http.Client, query string) (int, <-chan sse) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/events"+query, nil)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	out := make(chan sse, 64)
	if res.StatusCode != http.StatusOK {
		close(out)
		return res.StatusCode, out
	}
	go func() {
		defer close(out)
		sc := bufio.NewScanner(res.Body)
		var event string
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "event: "); ok {
				event = v
			} else if v, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				out <- sse{event, v}
			}
		}
	}()
	select {
	case ev := <-out:
		if ev.event != "ready" {
			t.Fatalf("first event: %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never opened")
	}
	time.Sleep(200 * time.Millisecond)
	return http.StatusOK, out
}

// next returns the next event named name within d, skipping others.
func next(t *testing.T, events <-chan sse, name string, d time.Duration) (sse, bool) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return sse{}, false
			}
			if ev.event == name {
				return ev, true
			}
		case <-deadline:
			return sse{}, false
		}
	}
}

func ticketIDs(t *testing.T, ev sse) []int64 {
	t.Helper()
	var body struct{ Tickets []int64 }
	if err := json.Unmarshal([]byte(ev.data), &body); err != nil {
		t.Fatalf("tickets event %q: %v", ev.data, err)
	}
	return body.Tickets
}

// A member's open board hears a ticket change within a second; a burst of 50
// arrives merged; the bell's notifications ride the same stream.
func TestEventsStreamTicketChanges(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime rounding", &w.a, w.ot)
	_, events := openEvents(t, e, w.pm, "?project=HRIS")

	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "On it."}, nil); code != http.StatusCreated {
		t.Fatalf("comment: %d", code)
	}
	ev, ok := next(t, events, "tickets", 2*time.Second)
	if !ok || !slices.Contains(ticketIDs(t, ev), tk.ID) {
		t.Fatalf("no tickets event for %d: %+v", tk.ID, ev)
	}

	for i := range 50 {
		if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET title = $2 WHERE id = $1", tk.ID, fmt.Sprintf("Burst %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for {
		if _, ok := next(t, events, "tickets", time.Second); !ok {
			break
		}
		count++
	}
	if count == 0 || count > 3 {
		t.Fatalf("50 updates arrived as %d events", count)
	}

	rina, rinaUser := e.signedIn("rina@example.com", false)
	e.seedMember(rinaUser, w.p, "member", w.a)
	_, rinaEvents := openEvents(t, e, rina, "?project=HRIS")
	e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "bug", "title": "Assigned live", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "assignee_id": rinaUser.ID,
	}, nil)
	if _, ok := next(t, rinaEvents, "notification", 5*time.Second); !ok {
		t.Fatal("no notification on /events")
	}
}

// A member scoped to client A hears core tickets and client A's, never
// client B's.
func TestEventsFilterByClientScope(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e) // the PM sees client A only
	core := e.seedTicket(w.p, w.pmUser, "Core rounding", nil, w.ot)
	other := e.seedTicket(w.p, w.pmUser, "Client B report", &w.b, w.secret)
	_, events := openEvents(t, e, w.pm, "?project=HRIS")
	for _, id := range []int64{other.ID, core.ID} {
		if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET title = title || '.' WHERE id = $1", id); err != nil {
			t.Fatal(err)
		}
	}
	var seen []int64
	for {
		ev, ok := next(t, events, "tickets", time.Second)
		if !ok {
			break
		}
		seen = append(seen, ticketIDs(t, ev)...)
	}
	if !slices.Contains(seen, core.ID) || slices.Contains(seen, other.ID) {
		t.Fatalf("seen %v; want core %d, never %d", seen, core.ID, other.ID)
	}
}

// Viewers may watch the board, archived project or not; strangers and unknown
// keys get 404, like the board.
func TestEventsStreamForViewers(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime rounding", &w.a, w.ot)
	viewer, viewerUser := e.signedIn("viewer@example.com", false)
	e.seedMember(viewerUser, w.p, "viewer")
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE projects SET archived_at = now() WHERE id = $1", w.p.ID); err != nil {
		t.Fatal(err)
	}
	_, events := openEvents(t, e, viewer, "?project=HRIS")
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET title = 'Seen by a viewer' WHERE id = $1", tk.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := next(t, events, "tickets", 2*time.Second); !ok {
		t.Fatal("a viewer heard nothing")
	}

	stranger, _ := e.signedIn("stranger@example.com", false)
	if code, _ := openEvents(t, e, stranger, "?project=HRIS"); code != http.StatusNotFound {
		t.Fatalf("stranger: %d", code)
	}
	if code, _ := openEvents(t, e, w.pm, "?project=NOPE"); code != http.StatusNotFound {
		t.Fatalf("unknown key: %d", code)
	}
}

// After the listener reconnects, every open stream is told to resync.
func TestEventsResyncAfterReconnect(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	_, events := openEvents(t, e, w.pm, "?project=HRIS")
	if _, err := e.d.Pool.Exec(context.Background(),
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE query LIKE 'LISTEN zettra_notifications%' AND pid <> pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	if _, ok := next(t, events, "resync", 5*time.Second); !ok {
		t.Fatal("no resync after the listener reconnected")
	}
}
