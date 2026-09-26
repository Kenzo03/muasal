package ticketimport

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// BatchSize is how many rows one transaction writes (§14.2).
const BatchSize = 500

// ImportTickets runs one import in the background, in batches of 500 rows.
// A retry resumes after the last batch written; rows written twice are
// harmless, as imports are idempotent (R-IN-1).
type ImportTickets struct {
	RunID int64 `json:"run_id"`
}

func (ImportTickets) Kind() string { return "import_tickets" }

func (ImportTickets) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 5}
}

// Worker is the job's worker; `app serve` registers it.
type Worker struct {
	river.WorkerDefaults[ImportTickets]
	Pool *pgxpool.Pool
}

// Timeout gives a 200 MB file time; each batch commits on its own.
func (w *Worker) Timeout(*river.Job[ImportTickets]) time.Duration { return 2 * time.Hour }

func (w *Worker) Work(ctx context.Context, job *river.Job[ImportTickets]) error {
	err := Run(ctx, w.Pool, job.Args.RunID, func(ctx context.Context, tx pgx.Tx, ids []int64) error {
		if len(ids) == 0 {
			return nil
		}
		params := make([]river.InsertManyParams, len(ids))
		for i, id := range ids {
			params[i] = river.InsertManyParams{Args: indexer.IndexTicket{TicketID: id}}
		}
		_, err := river.ClientFromContext[pgx.Tx](ctx).InsertManyTx(ctx, tx, params)
		return err
	})
	if err != nil && job.Attempt >= job.MaxAttempts {
		q := db.New(w.Pool)
		if run, gerr := q.GetImportRun(ctx, job.Args.RunID); gerr == nil {
			_ = q.SetImportStatus(ctx, db.SetImportStatusParams{ID: run.ImportRun.ID, Status: "failed", Stats: run.ImportRun.Stats})
		}
	}
	return err
}

// Run writes an import's rows, batch by batch, each batch with its history
// and its index jobs (queued by index) in one transaction.
func Run(ctx context.Context, pool *pgxpool.Pool, runID int64, index func(context.Context, pgx.Tx, []int64) error) error {
	q := db.New(pool)
	run, err := q.GetImportRun(ctx, runID)
	if err != nil {
		return err
	}
	r := run.ImportRun
	var m Mapping
	var st Stats
	if err := errors.Join(json.Unmarshal(r.Mapping, &m), json.Unmarshal(r.Stats, &st)); err != nil {
		return err
	}
	p, err := LoadProject(ctx, q, r.ProjectID)
	if err != nil {
		return err
	}
	f, err := os.Open(r.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	wr := Writer{Project: p, Mapping: m, Importer: r.CreatedBy}
	var batch []Record
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			tq := q.WithTx(tx)
			var ids []int64
			for _, rec := range batch {
				w, err := wr.Write(ctx, tq, rec)
				if err != nil {
					return err
				}
				st.Done++
				st.LastLine = rec.Line
				if w == nil {
					continue
				}
				st.Tickets++
				st.Comments += w.Comments
				ids = append(ids, w.TicketID)
				action := "import_update"
				if w.Created {
					action = "import_create"
				}
				changes, _ := json.Marshal(map[string]any{"external_ref": rec.Key, "import_run": runID})
				if err := tq.InsertAuditEvent(ctx, db.InsertAuditEventParams{
					ActorID: &r.CreatedBy, Via: "import", Entity: "ticket", EntityID: w.TicketID, ProjectID: &p.ID, Action: action, Changes: changes,
				}); err != nil {
					return err
				}
			}
			if err := index(ctx, tx, ids); err != nil {
				return err
			}
			stats, _ := json.Marshal(st)
			batch = batch[:0]
			return tq.SetImportStatus(ctx, db.SetImportStatusParams{ID: runID, Status: "running", Stats: stats})
		})
	}
	resumeAfter := st.LastLine
	err = Read(f, m, func(rec Record) error {
		if rec.Line <= resumeAfter {
			return nil
		}
		batch = append(batch, rec)
		if len(batch) >= BatchSize {
			return flush()
		}
		return nil
	})
	if err == nil {
		err = flush()
	}
	if err != nil {
		return err
	}
	stats, _ := json.Marshal(st)
	return q.SetImportStatus(ctx, db.SetImportStatusParams{ID: runID, Status: "done", Stats: stats})
}
