package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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
