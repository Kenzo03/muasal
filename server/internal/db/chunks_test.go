package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// FSD §13.2 at the database: an unchanged chunk keeps its vector, a changed
// one waits for the embedder, a shorter source loses its tail, and the counts
// per model feed Index status.
func TestChunksKeepVectorsUntilTheirTextChanges(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	todo := must(q.ListStatuses(ctx, p.ID))[0]
	tk := must(q.CreateTicket(ctx, db.CreateTicketParams{
		ProjectID: p.ID, Number: must(q.NextTicketNumber(ctx, p.ID)), Key: "HRIS-1", Type: "bug", Title: "A ticket",
		StatusID: todo.ID, RequesterUserID: &u.ID, ReporterID: u.ID, Priority: "medium",
	}))
	chunk := func(seq int32, content string) db.UpsertChunkParams {
		return db.UpsertChunkParams{
			SourceType: "ticket", SourceID: tk.ID, Seq: seq, TicketID: tk.ID, ProjectID: p.ID,
			NodeIds: []int64{}, UserIds: []int64{u.ID}, ContactIds: []int64{}, OccurredAt: time.Now(),
			Content: content, ContentHash: []byte(content),
		}
	}
	check(q.UpsertChunk(ctx, chunk(0, "part one")))
	check(q.UpsertChunk(ctx, chunk(1, "part two")))
	pending := must(q.ListPendingChunks(ctx, db.ListPendingChunksParams{Model: "bge-m3", Lim: 10}))
	if len(pending) != 2 {
		t.Fatalf("pending: %d", len(pending))
	}
	vec := pgvector.NewHalfVector(make([]float32, 1024))
	for _, c := range pending {
		check(q.SetChunkEmbedding(ctx, db.SetChunkEmbeddingParams{ID: c.ID, Embedding: vec, Model: "bge-m3", ContentHash: c.ContentHash}))
	}

	check(q.UpsertChunk(ctx, chunk(0, "part one")))         // unchanged: keeps its vector
	check(q.UpsertChunk(ctx, chunk(1, "part two, edited"))) // changed: waits for the embedder
	got := must(q.ListTicketChunks(ctx, tk.ID))
	if len(got) != 2 || !got[0].Embedded || got[1].Embedded {
		t.Fatalf("after re-upsert: %+v", got)
	}
	if n := must(q.CountPendingChunks(ctx, "bge-m3")); n != 1 {
		t.Fatalf("pending after the edit: %d", n)
	}
	if n := must(q.CountPendingChunks(ctx, "another-model")); n != 2 {
		t.Fatalf("pending for a new embedding model: %d", n)
	}

	check(q.DeleteChunksFrom(ctx, db.DeleteChunksFromParams{SourceType: "ticket", SourceID: tk.ID, Seq: 1}))
	counts := must(q.CountChunksByModel(ctx))
	if len(counts) != 1 || counts[0].Model != "bge-m3" || counts[0].Chunks != 1 {
		t.Fatalf("counts: %+v", counts)
	}
	var stored []float32
	if err := d.Pool.QueryRow(ctx, "SELECT embedding FROM chunks").Scan(ptrTo(&stored)); err != nil || len(stored) != 1024 {
		t.Fatalf("stored vector: %d %v", len(stored), err)
	}
}

// ptrTo scans a halfvec column into a []float32 through pgvector's scanner.
func ptrTo(dst *[]float32) *halfvecScanner { return &halfvecScanner{dst} }

type halfvecScanner struct{ dst *[]float32 }

func (s *halfvecScanner) Scan(src any) error {
	var v pgvector.HalfVector
	if err := v.Scan(src); err != nil {
		return err
	}
	*s.dst = v.Slice()
	return nil
}
