package indexer_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/zettra/server/internal/ai"
	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/indexer"
	"github.com/kenzo03/zettra/server/internal/llm/llmtest"
	"github.com/kenzo03/zettra/server/internal/testdb"
)

type world struct {
	d      testdb.DB
	q      *db.Queries
	rt     *ai.Runtime
	fake   *llmtest.Server
	admin  db.User
	ticket db.Ticket
}

// newWorld seeds one ticket on Overtime Approval with one comment, and sets AI
// to Local against a fake model server.
func newWorld(t *testing.T) *world {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	w := &world{d: d, q: q, fake: llmtest.New(t)}
	w.rt = &ai.Runtime{Store: ai.NewStore(q), Gate: ai.NewGate()}
	var err error
	must := func(e error) {
		if e != nil {
			t.Fatal(e)
		}
	}
	w.admin, err = q.CreateUser(ctx, db.CreateUserParams{Email: "rina@example.com", Name: "Rina", Locale: "id", Timezone: "Asia/Jakarta", IsAdmin: true})
	must(err)
	p, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"})
	must(err)
	statuses, err := q.ListStatuses(ctx, p.ID)
	must(err)
	n, err := q.NextTicketNumber(ctx, p.ID)
	must(err)
	w.ticket, err = q.CreateTicket(ctx, db.CreateTicketParams{
		ProjectID: p.ID, Number: n, Key: "HRIS-1", Type: "change_request", Title: "Skip supervisor approval",
		StatusID: statuses[0].ID, RequesterUserID: &w.admin.ID, ReporterID: w.admin.ID, Priority: "medium",
		Reason: "Supervisors are often on leave.",
	})
	must(err)
	_, err = d.Pool.Exec(ctx, `INSERT INTO comments (ticket_id, author_id, body) VALUES ($1, $2, 'Confirmed by phone.')`, w.ticket.ID, w.admin.ID)
	must(err)
	w.setMode(t, ai.ModeLocal)
	return w
}

func (w *world) setMode(t *testing.T, mode ai.Mode) {
	s := ai.Defaults()
	s.Mode, s.Chat.URL, s.Embed.URL = mode, w.fake.BaseURL(), w.fake.BaseURL()
	if err := w.rt.Store.Put(context.Background(), w.q, s, w.admin.ID); err != nil {
		t.Fatal(err)
	}
}

func (w *world) chunks(t *testing.T) []db.ListTicketChunksRow {
	rows, err := w.q.ListTicketChunks(context.Background(), w.ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// AC-IX-3: five quick edits leave only the final content, and unchanged
// chunks are not embedded again.
func TestRebuildEmbedsOnlyChangedChunks(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	ix := indexer.New(w.d.Pool, w.rt)
	if err := ix.Rebuild(ctx, w.ticket.ID); err != nil {
		t.Fatal(err)
	}
	if err := ix.EmbedTicket(ctx, w.ticket.ID); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		if _, err := w.d.Pool.Exec(ctx, "UPDATE tickets SET title = $2 WHERE id = $1", w.ticket.ID, "Skip supervisor approval, take "+string(rune('0'+i))); err != nil {
			t.Fatal(err)
		}
		if err := ix.Rebuild(ctx, w.ticket.ID); err != nil {
			t.Fatal(err)
		}
		if err := ix.EmbedTicket(ctx, w.ticket.ID); err != nil {
			t.Fatal(err)
		}
	}
	rows := w.chunks(t)
	if len(rows) != 2 || !strings.Contains(rows[1].Content, "take 5") || !rows[0].Embedded || !rows[1].Embedded {
		t.Fatalf("chunks: %+v", rows)
	}
	if n := len(w.fake.Embedded); n != 7 { // header and comment once, then the header five times
		t.Fatalf("embedded %d texts, want 7", n)
	}
}

// R-AI-5: in Off mode the chunks are written without vectors and nothing is
// called; switching AI on embeds them.
func TestOffModeWritesChunksWithoutVectors(t *testing.T) {
	w := newWorld(t)
	w.setMode(t, ai.ModeOff)
	ctx := context.Background()
	ix := indexer.New(w.d.Pool, w.rt)
	if err := ix.Rebuild(ctx, w.ticket.ID); err != nil {
		t.Fatal(err)
	}
	if err := ix.EmbedTicket(ctx, w.ticket.ID); err != nil {
		t.Fatal(err)
	}
	if rows := w.chunks(t); len(rows) != 2 || rows[0].Embedded || len(w.fake.Embedded) != 0 {
		t.Fatalf("off: %+v, embedded %d", rows, len(w.fake.Embedded))
	}
	w.setMode(t, ai.ModeLocal)
	if err := ix.EmbedPending(ctx); err != nil {
		t.Fatal(err)
	}
	if rows := w.chunks(t); !rows[0].Embedded || !rows[1].Embedded {
		t.Fatalf("after switching on: %+v", rows)
	}
}

// AC-IX-2: with the model server stopped the job keeps its chunks for keyword
// search and retries; once the server is back, the backlog drains on its own.
func TestJobsSurviveAStoppedModelServer(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.fake.Set(func(s *llmtest.Server) { s.Down = true })
	client, err := indexer.NewClient(w.d.Pool, w.rt, slog.New(slog.NewTextHandler(io.Discard, nil)), indexer.Options{PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(ctx)
	if _, err := client.Insert(ctx, indexer.IndexTicket{TicketID: w.ticket.ID}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a failed attempt", func() bool {
		var state string
		_ = w.d.Pool.QueryRow(ctx, "SELECT state FROM river_job WHERE kind = 'index_ticket'").Scan(&state)
		return state == "retryable"
	})
	if rows := w.chunks(t); len(rows) != 2 || rows[0].Embedded {
		t.Fatalf("chunks while the server is down: %+v", rows)
	}
	w.fake.Set(func(s *llmtest.Server) { s.Down = false })
	if _, err := client.Insert(ctx, indexer.EmbedPending{}, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the backlog to drain", func() bool {
		rows := w.chunks(t)
		return rows[0].Embedded && rows[1].Embedded
	})
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}
