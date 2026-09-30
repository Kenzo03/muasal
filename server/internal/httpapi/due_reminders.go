package httpapi

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/indexer"
)

// DueReminders asks for the morning's due-date reminders; `app serve` runs it
// hourly, so each user's morning comes in their own timezone (MSL-52).
type DueReminders struct{}

func (DueReminders) Kind() string { return "due_reminders" }

func (DueReminders) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 3}
}

// DueRemindersWorker writes the reminders; a user hears about a ticket once a day.
type DueRemindersWorker struct {
	river.WorkerDefaults[DueReminders]
	Server *Server
}

func (w *DueRemindersWorker) Work(ctx context.Context, _ *river.Job[DueReminders]) error {
	_, err := w.Server.q.NotifyDue(ctx, w.Server.now())
	return err
}

// RemindDue writes the reminders now; tests use it.
func (s *Server) RemindDue(ctx context.Context, now time.Time) (int, error) {
	rows, err := s.q.NotifyDue(ctx, now)
	return len(rows), err
}
