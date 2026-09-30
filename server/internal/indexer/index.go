package indexer

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
)

// IndexTicket rebuilds one ticket's chunks: its header, comments and decision
// record. The unit is the ticket, because comments and decisions carry the
// ticket's filter columns (§13.1). Mutations queue it in their transaction.
type IndexTicket struct {
	TicketID int64 `json:"ticket_id"`
}

func (IndexTicket) Kind() string { return "index_ticket" }

// InsertOpts: failures retry with backoff, up to 10 attempts (§13.2).
func (IndexTicket) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, MaxAttempts: 10}
}

// EmbedPending embeds chunks that lack a vector from the current embedding
// model: after the model server was down, after Off → Local, or after an
// embedding-model change (§13.4, R-AI-5). It runs every minute; one waits at a time.
type EmbedPending struct{}

func (EmbedPending) Kind() string { return "embed_pending" }

func (EmbedPending) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, MaxAttempts: 10, UniqueOpts: river.UniqueOpts{ByState: []rivertype.JobState{
		rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
		rivertype.JobStateRetryable, rivertype.JobStateScheduled,
	}}}
}

// QueueIndex is the queue of every index job.
const QueueIndex = "index"

// batchSize is how many chunks one embeddings call carries (§13.2).
const batchSize = 32

// Indexer does the work behind the jobs; tests call it directly.
type Indexer struct {
	pool *pgxpool.Pool
	q    *db.Queries
	ai   *ai.Runtime
}

// New returns an Indexer over pool that embeds as rt's settings say.
func New(pool *pgxpool.Pool, rt *ai.Runtime) *Indexer {
	return &Indexer{pool: pool, q: db.New(pool), ai: rt}
}

// Rebuild writes a ticket's chunks in one transaction under a per-ticket
// advisory lock. Unchanged chunks keep their vectors; changed ones wait for
// the embedder; chunks the ticket no longer has are removed (§13.2).
func (ix *Indexer) Rebuild(ctx context.Context, ticketID int64) error {
	tx, err := ix.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := ix.q.WithTx(tx)
	if err := q.LockTicketIndex(ctx, ticketID); err != nil {
		return err
	}
	src, err := Load(ctx, q, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := q.DeleteTicketChunks(ctx, ticketID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	chunks := Build(src)
	keep := make([]string, len(chunks))
	for i, c := range chunks {
		if err := q.UpsertChunk(ctx, c); err != nil {
			return err
		}
		keep[i] = Key(c)
	}
	if err := q.DeleteStaleChunks(ctx, db.DeleteStaleChunksParams{TicketID: ticketID, Keep: keep}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Load reads a ticket as its chunks describe it; Ask packs evidence from it too.
func Load(ctx context.Context, q *db.Queries, ticketID int64) (Source, error) {
	t, err := q.GetTicketSource(ctx, ticketID)
	if err != nil {
		return Source{}, err
	}
	src := Source{Ticket: t}
	if src.Menus, err = q.ListTicketNodePaths(ctx, ticketID); err != nil {
		return Source{}, err
	}
	if src.Comments, err = q.ListCommentSources(ctx, ticketID); err != nil {
		return Source{}, err
	}
	if src.Commits, err = q.ListTicketCommits(ctx, ticketID); err != nil {
		return Source{}, err
	}
	if src.MRs, err = q.ListTicketMergeRequests(ctx, ticketID); err != nil {
		return Source{}, err
	}
	if src.Links, err = q.ListTicketLinks(ctx, ticketID); err != nil {
		return Source{}, err
	}
	var emails []string
	for _, c := range src.Commits {
		if c.AuthorEmail != nil {
			emails = append(emails, strings.ToLower(*c.AuthorEmail))
		}
	}
	if len(emails) > 0 {
		if src.Coders, err = q.UserIDsByEmails(ctx, emails); err != nil {
			return Source{}, err
		}
	}
	d, err := q.GetDecision(ctx, ticketID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Source{}, err
	case d.DecisionRecord.State == "confirmed":
		src.Decision = &d
	}
	return src, nil
}

// EmbedTicket embeds the ticket's chunks that lack a current vector. In Off
// mode it does nothing: the chunks serve keyword search until AI is on (R-AI-5).
func (ix *Indexer) EmbedTicket(ctx context.Context, ticketID int64) error {
	return ix.embed(ctx, func(model string) ([]pending, error) {
		rows, err := ix.q.ListPendingTicketChunks(ctx, db.ListPendingTicketChunksParams{TicketID: ticketID, Model: model})
		out := make([]pending, len(rows))
		for i, r := range rows {
			out[i] = pending{r.ID, r.Content, r.ContentHash}
		}
		return out, err
	}, false)
}

// EmbedPending embeds every chunk without a current vector, newest first.
func (ix *Indexer) EmbedPending(ctx context.Context) error {
	return ix.embed(ctx, func(model string) ([]pending, error) {
		rows, err := ix.q.ListPendingChunks(ctx, db.ListPendingChunksParams{Model: model, Lim: batchSize})
		out := make([]pending, len(rows))
		for i, r := range rows {
			out[i] = pending{r.ID, r.Content, r.ContentHash}
		}
		return out, err
	}, true)
}

type pending struct {
	id      int64
	content string
	hash    []byte
}

// embed takes batches from next until it returns none. With untilEmpty false,
// next is called once and may return more than one batch.
func (ix *Indexer) embed(ctx context.Context, next func(model string) ([]pending, error), untilEmpty bool) error {
	s, err := ix.ai.Store.Get(ctx)
	if err != nil {
		return err
	}
	c, err := ix.ai.EmbedClient(s)
	if errors.Is(err, ai.ErrOff) {
		return nil
	}
	if err != nil {
		return err
	}
	for {
		rows, err := next(s.Embed.Model)
		if err != nil || len(rows) == 0 {
			return err
		}
		for start := 0; start < len(rows); start += batchSize {
			batch := rows[start:min(start+batchSize, len(rows))]
			// Generations go first: embedding waits while anyone waits for an answer (§11.7).
			if err := ix.ai.Gate.WaitIdle(ctx); err != nil {
				return err
			}
			texts := make([]string, len(batch))
			for i, r := range batch {
				texts[i] = r.content
			}
			vecs, err := c.Embed(ctx, texts)
			if err != nil {
				return err
			}
			for i, r := range batch {
				if len(vecs[i]) != s.EmbedDim {
					return fmt.Errorf("%s returns %d dimensions, the index holds %d: run Test connection and save to re-index", s.Embed.Model, len(vecs[i]), s.EmbedDim)
				}
				if err := ix.q.SetChunkEmbedding(ctx, db.SetChunkEmbeddingParams{
					ID: r.id, Embedding: pgvector.NewHalfVector(vecs[i]), Model: s.Embed.Model, ContentHash: r.hash,
				}); err != nil {
					return err
				}
			}
		}
		if !untilEmpty {
			return nil
		}
	}
}

type indexWorker struct {
	river.WorkerDefaults[IndexTicket]
	ix *Indexer
}

func (w *indexWorker) Work(ctx context.Context, job *river.Job[IndexTicket]) error {
	if err := w.ix.Rebuild(ctx, job.Args.TicketID); err != nil {
		return err
	}
	return w.ix.EmbedTicket(ctx, job.Args.TicketID)
}

type embedWorker struct {
	river.WorkerDefaults[EmbedPending]
	ix *Indexer
}

func (w *embedWorker) Work(ctx context.Context, job *river.Job[EmbedPending]) error {
	return w.ix.EmbedPending(ctx)
}

// Timeout gives a long backlog time to drain; each batch is short.
func (w *embedWorker) Timeout(*river.Job[EmbedPending]) time.Duration { return 30 * time.Minute }

// Options tune the worker client; tests poll faster.
type Options struct {
	PollInterval time.Duration        // default 1 s
	Workers      int                  // default 4
	OwnerURL     string               // MIGRATE_DATABASE_URL, for ChangeDimension
	AskLogDays   int                  // Ask log retention in days; 0 keeps it (FSD §15.4)
	Register     func(*river.Workers) // adds other packages' workers, such as ticket imports
	Periodic     []*river.PeriodicJob // other packages' periodic jobs, such as weekly summaries
}

// NewClient returns the River client that `app serve` starts: the index queue's
// workers plus EmbedPending every minute.
func NewClient(pool *pgxpool.Pool, rt *ai.Runtime, log *slog.Logger, opts Options) (*river.Client[pgx.Tx], error) {
	ix := New(pool, rt)
	workers := river.NewWorkers()
	river.AddWorker(workers, &indexWorker{ix: ix})
	river.AddWorker(workers, &embedWorker{ix: ix})
	river.AddWorker(workers, &noteWorker{ix: ix})
	river.AddWorker(workers, &sectionWorker{ix: ix})
	river.AddWorker(workers, &dimensionWorker{ownerURL: opts.OwnerURL})
	river.AddWorker(workers, &purgeWorker{pool: pool, days: opts.AskLogDays})
	if opts.Register != nil {
		opts.Register(workers)
	}
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:            log,
		FetchPollInterval: cmp.Or(opts.PollInterval, time.Second),
		Queues:            map[string]river.QueueConfig{QueueIndex: {MaxWorkers: cmp.Or(opts.Workers, 4)}},
		Workers:           workers,
		MaxAttempts:       10,
		PeriodicJobs: append([]*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return EmbedPending{}, nil }, &river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(24*time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return PurgeAsk{}, nil }, &river.PeriodicJobOpts{RunOnStart: true}),
		}, opts.Periodic...),
	})
}
