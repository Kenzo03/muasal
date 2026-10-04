package httpapi_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
)

// startWorkers runs the index workers against the test database, sharing the
// API's AI runtime as `app serve` does.
func (e *env) startWorkers() {
	e.t.Helper()
	c, err := indexer.NewClient(e.d.Pool, e.api.AI(), slog.New(slog.NewTextHandler(io.Discard, nil)),
		indexer.Options{PollInterval: 100 * time.Millisecond, OwnerURL: e.d.OwnerURL})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Stop(ctx)
	})
}

// eventually polls ok for up to 15 seconds.
func (e *env) eventually(what string, ok func() bool) {
	e.t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if ok() {
			return
		}
	}
	e.t.Fatalf("timed out waiting for %s", what)
}

func (e *env) status(admin *http.Client) httpapi.IndexStatus {
	e.t.Helper()
	var st httpapi.IndexStatus
	if code := e.call(admin, http.MethodGet, "/admin/ai/status", nil, &st); code != http.StatusOK {
		e.t.Fatalf("status: %d", code)
	}
	return st
}

// AC-IX-4: a new 768-dimension embedding model needs a confirmation, then the
// column changes and every chunk is embedded again; Index status ends with
// 100% of chunks on the new model.
func TestEmbeddingModelChangeReembedsEverything(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	fake := llmtest.New(t)
	e.startWorkers()
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", aiUpdate("local", fake.BaseURL()), nil); code != http.StatusOK {
		t.Fatalf("switch to Local: %d", code)
	}
	for _, title := range []string{"Overtime cap of 40 hours", "Overtime export for payroll"} {
		e.seedTicket(w.p, w.pmUser, title, &w.a, w.ot)
	}
	var queued httpapi.ReindexResult
	if code := e.call(admin, http.MethodPost, "/admin/ai/reindex", map[string]any{"scope": "all"}, &queued); code != http.StatusAccepted || queued.Queued != 2 {
		t.Fatalf("re-index all: %d %+v", code, queued)
	}
	e.eventually("the first embedding", func() bool {
		st := e.status(admin)
		return st.TotalChunks == 2 && st.PendingChunks == 0 && st.QueuedJobs == 0
	})

	fake.Set(func(s *llmtest.Server) { s.Dim = 768 })
	body := aiUpdate("local", fake.BaseURL())
	body["embed"] = map[string]any{"url": fake.BaseURL(), "model": "nomic-embed-text"}
	body["embed_dim"] = 768
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "reindex_required" {
		t.Fatalf("a new model without confirming: %d %+v", code, p)
	}
	body["reindex"] = true
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, nil); code != http.StatusOK {
		t.Fatalf("a new model, confirmed: %d", code)
	}
	e.eventually("the re-embedding", func() bool {
		st := e.status(admin)
		return st.EmbedModel == "nomic-embed-text" && st.PendingChunks == 0 && len(st.ChunksByModel) == 1 &&
			st.ChunksByModel[0].Model == "nomic-embed-text" && st.ChunksByModel[0].Chunks == 2
	})
	var column string
	if err := e.d.Pool.QueryRow(context.Background(), `SELECT format_type(atttypid, atttypmod) FROM pg_attribute
		WHERE attrelid = 'chunks'::regclass AND attname = 'embedding'`).Scan(&column); err != nil || column != "halfvec(768)" {
		t.Fatalf("column: %q %v", column, err)
	}
}

// §13.2: a job that used up its attempts shows under Index status and can be retried.
func TestFailedIndexJobsCanBeRetried(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	fake := llmtest.New(t)
	e.startWorkers()
	e.call(admin, http.MethodPut, "/admin/settings/ai", aiUpdate("local", fake.BaseURL()), nil)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime cap of 40 hours", &w.a, w.ot)
	fake.Set(func(s *llmtest.Server) { s.Down, s.DownBody = true, "SECRET" })
	if _, err := e.d.Pool.Exec(context.Background(), `INSERT INTO river_job (args, kind, max_attempts, queue, state)
		VALUES (jsonb_build_object('ticket_id', $1::bigint), 'index_ticket', 1, 'index', 'available')`, tk.ID); err != nil {
		t.Fatal(err)
	}
	e.eventually("the job to fail for good", func() bool { return len(e.status(admin).FailedJobs) == 1 })
	st := e.status(admin)
	if f := st.FailedJobs[0]; f.TicketId != tk.ID || f.Attempts != 1 || f.Error == "" || strings.Contains(f.Error, "SECRET") {
		t.Fatalf("failed job: %+v", f)
	}
	fake.Set(func(s *llmtest.Server) { s.Down = false })
	var res httpapi.ReindexResult
	if code := e.call(admin, http.MethodPost, "/admin/ai/reindex", map[string]any{"scope": "failed"}, &res); code != http.StatusAccepted || res.Queued != 1 {
		t.Fatalf("retry: %d %+v", code, res)
	}
	e.eventually("the retried job", func() bool {
		st := e.status(admin)
		return st.TotalChunks == 1 && st.PendingChunks == 0
	})
}
