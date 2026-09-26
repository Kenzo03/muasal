package indexer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
)

// ChangeDimension switches the chunks column to a new embedding dimension:
// it drops the vector index, clears every vector, changes the column type,
// rebuilds the index and queues EmbedPending (§13.4). Keyword search keeps
// working throughout. ALTER TABLE needs the owner role, so the job connects
// with MIGRATE_DATABASE_URL.
type ChangeDimension struct {
	Dim int `json:"dim"`
}

func (ChangeDimension) Kind() string { return "change_dimension" }

func (ChangeDimension) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, MaxAttempts: 3}
}

type dimensionWorker struct {
	river.WorkerDefaults[ChangeDimension]
	ownerURL string
}

func (w *dimensionWorker) Work(ctx context.Context, job *river.Job[ChangeDimension]) error {
	if w.ownerURL == "" {
		return errors.New("MIGRATE_DATABASE_URL is not set, so the embedding dimension cannot change")
	}
	if err := SetDimension(ctx, w.ownerURL, job.Args.Dim); err != nil {
		return err
	}
	_, err := river.ClientFromContext[pgx.Tx](ctx).Insert(ctx, EmbedPending{}, nil)
	return err
}

// Timeout covers rebuilding the index on a large install.
func (w *dimensionWorker) Timeout(*river.Job[ChangeDimension]) time.Duration { return time.Hour }

// SetDimension changes the embedding column to dim as the owner role; the
// CLI's reindex uses it too.
func SetDimension(ctx context.Context, ownerURL string, dim int) error {
	if dim < 1 || dim > 4000 {
		return fmt.Errorf("embedding dimension %d is outside 1–4000", dim)
	}
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, stmt := range []string{
		"DROP INDEX IF EXISTS chunks_vec_idx",
		"UPDATE chunks SET embedding = NULL, embed_model = NULL",
		fmt.Sprintf("ALTER TABLE chunks ALTER COLUMN embedding TYPE halfvec(%d)", dim),
		"CREATE INDEX chunks_vec_idx ON chunks USING hnsw (embedding halfvec_cosine_ops) WITH (m = 16, ef_construction = 64)",
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return tx.Commit(ctx)
}

// QueueAll queues an index job for every ticket and decision note, newest
// first, and returns how many (`app admin reindex --all`, Admin → AI →
// Re-index all).
func QueueAll(ctx context.Context, pool *pgxpool.Pool, client *river.Client[pgx.Tx]) (int, error) {
	q := db.New(pool)
	ids, err := q.ListAllTicketIDs(ctx)
	if err != nil {
		return 0, err
	}
	notes, err := q.ListAllNoteIDs(ctx)
	if err != nil {
		return 0, err
	}
	sections, err := q.ListAllSectionIDs(ctx)
	if err != nil {
		return 0, err
	}
	args := make([]river.JobArgs, 0, len(ids)+len(notes)+len(sections))
	for _, id := range ids {
		args = append(args, IndexTicket{TicketID: id})
	}
	for _, id := range notes {
		args = append(args, IndexNote{NoteID: id})
	}
	for _, id := range sections {
		args = append(args, IndexSection{SectionID: id})
	}
	for start := 0; start < len(args); start += 1000 {
		batch := args[start:min(start+1000, len(args))]
		params := make([]river.InsertManyParams, len(batch))
		for i, a := range batch {
			params[i] = river.InsertManyParams{Args: a}
		}
		if _, err := client.InsertMany(ctx, params); err != nil {
			return start, err
		}
	}
	return len(args), nil
}

// Status is Index status (§13.3).
type Status struct {
	Total, Pending, Queued int64
	ByModel                []db.CountChunksByModelRow
	Failed                 []FailedJob
	LastIndexedAt          *time.Time
}

// FailedJob is an index job that used up its attempts.
type FailedJob struct {
	ID, TicketID int64
	Attempts     int
	Error        string
	At           time.Time
}

// ReadStatus counts chunks per model and pending for model, and reads River's
// index jobs: waiting or retrying, and the 50 newest that failed for good.
func ReadStatus(ctx context.Context, pool *pgxpool.Pool, model string) (Status, error) {
	q := db.New(pool)
	var st Status
	var err error
	if st.ByModel, err = q.CountChunksByModel(ctx); err != nil {
		return st, err
	}
	for _, m := range st.ByModel {
		st.Total += m.Chunks
		if st.LastIndexedAt == nil || m.Latest.After(*st.LastIndexedAt) {
			latest := m.Latest
			st.LastIndexedAt = &latest
		}
	}
	if st.Pending, err = q.CountPendingChunks(ctx, model); err != nil {
		return st, err
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM river_job
		WHERE queue = $1 AND state IN ('available', 'pending', 'retryable', 'running', 'scheduled')`, QueueIndex).Scan(&st.Queued); err != nil {
		return st, err
	}
	rows, err := pool.Query(ctx, `SELECT id, coalesce((args->>'ticket_id')::bigint, 0), attempt,
		coalesce(errors[array_upper(errors, 1)]->>'error', ''), coalesce(finalized_at, attempted_at, created_at)
		FROM river_job WHERE queue = $1 AND state = 'discarded' ORDER BY id DESC LIMIT 50`, QueueIndex)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var f FailedJob
		if err := rows.Scan(&f.ID, &f.TicketID, &f.Attempts, &f.Error, &f.At); err != nil {
			return st, err
		}
		st.Failed = append(st.Failed, f)
	}
	return st, rows.Err()
}
