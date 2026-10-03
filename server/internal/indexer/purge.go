package indexer

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
)

// PurgeAskLog deletes questions older than the retention period, then the
// threads left empty, in one transaction (FSD §15.4). days 0 keeps everything.
func PurgeAskLog(ctx context.Context, pool *pgxpool.Pool, days int, now time.Time) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days)
	var n int64
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		if n, err = q.PurgeAskQueries(ctx, cutoff); err != nil {
			return err
		}
		_, err = q.PurgeEmptyThreads(ctx, cutoff)
		return err
	})
	return n, err
}

// PurgeAsk is the daily retention job; it also drops idempotency keys past
// their 24 hours (§17.1) and old MCP sign-in codes.
type PurgeAsk struct{}

func (PurgeAsk) Kind() string { return "purge_ask_log" }

func (PurgeAsk) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: time.Hour}}
}

type purgeWorker struct {
	river.WorkerDefaults[PurgeAsk]
	pool *pgxpool.Pool
	days int
}

func (w *purgeWorker) Work(ctx context.Context, _ *river.Job[PurgeAsk]) error {
	q := db.New(w.pool)
	if _, err := q.PurgeIdempotencyKeys(ctx); err != nil {
		return err
	}
	if _, err := q.PurgeNotifications(ctx); err != nil { // kept 90 days (§8.10)
		return err
	}
	if _, err := q.PurgeOAuthCodes(ctx); err != nil { // MCP sign-in codes, a day past expiry
		return err
	}
	_, err := PurgeAskLog(ctx, w.pool, w.days, time.Now())
	return err
}
