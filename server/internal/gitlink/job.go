package gitlink

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/indexer"
)

// ProcessDelivery stores what one webhook delivery names, after the webhook
// has answered (§14.1).
type ProcessDelivery struct {
	DeliveryID int64 `json:"delivery_id"`
}

func (ProcessDelivery) Kind() string { return "git_delivery" }

func (ProcessDelivery) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 5}
}

// Worker is the job's worker; `app serve` registers it.
type Worker struct {
	river.WorkerDefaults[ProcessDelivery]
	Pool *pgxpool.Pool
}

func (w *Worker) Work(ctx context.Context, job *river.Job[ProcessDelivery]) error {
	return Process(ctx, w.Pool, job.Args.DeliveryID, func(ctx context.Context, tx pgx.Tx, ids []int64) error {
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
}

// Process links a delivery's commits and merge request to the tickets their
// messages, titles, descriptions and branches name, re-indexes those tickets
// (queued by index) and drops the delivery, all in one transaction. A ticket
// key counts only when the ticket exists in the repository's project.
// Re-deliveries change nothing.
func Process(ctx context.Context, pool *pgxpool.Pool, deliveryID int64, index func(context.Context, pgx.Tx, []int64) error) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		d, err := q.GetDelivery(ctx, deliveryID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // processed already
		}
		if err != nil {
			return err
		}
		commits, mr, branch, err := Parse(d.Provider, d.Event, d.Payload)
		if err != nil {
			return q.DeleteDelivery(ctx, d.ID) // not a payload we read; nothing to retry
		}
		var touched []int64
		for _, c := range commits {
			ids, err := q.TicketIDsByKeys(ctx, db.TicketIDsByKeysParams{ProjectID: d.ProjectID, Keys: Keys(c.Message, branch)})
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				continue
			}
			p := db.UpsertCommitParams{RepoID: d.RepoID, Sha: c.SHA, Message: c.Message, CommittedAt: c.At}
			if c.AuthorName != "" {
				p.AuthorName = &c.AuthorName
			}
			if c.AuthorEmail != "" {
				p.AuthorEmail = &c.AuthorEmail
			}
			if c.URL != "" {
				p.Url = &c.URL
			}
			cid, err := q.UpsertCommit(ctx, p)
			if err != nil {
				return err
			}
			for _, id := range ids {
				if err := q.LinkCommit(ctx, db.LinkCommitParams{TicketID: id, CommitID: cid}); err != nil {
					return err
				}
				touched = append(touched, id)
			}
			if err := moveFixed(ctx, q, d.ProjectID, c); err != nil {
				return err
			}
		}
		if mr != nil && mr.Number > 0 {
			ids, err := q.TicketIDsByKeys(ctx, db.TicketIDsByKeysParams{ProjectID: d.ProjectID, Keys: Keys(mr.Title, mr.Body, mr.Branch)})
			if err != nil {
				return err
			}
			if len(ids) > 0 {
				p := db.UpsertMergeRequestParams{RepoID: d.RepoID, Number: int32(mr.Number), Title: mr.Title, State: mr.State, MergedAt: mr.MergedAt}
				if mr.URL != "" {
					p.Url = &mr.URL
				}
				mid, err := q.UpsertMergeRequest(ctx, p)
				if err != nil {
					return err
				}
				for _, id := range ids {
					if err := q.LinkMergeRequest(ctx, db.LinkMergeRequestParams{TicketID: id, MrID: mid}); err != nil {
						return err
					}
					touched = append(touched, id)
				}
			}
		}
		if err := index(ctx, tx, touched); err != nil {
			return err
		}
		return q.DeleteDelivery(ctx, d.ID)
	})
}

// moveFixed moves the tickets a commit says it fixes to review (MSL-29), with
// an audit event, so the ticket's history says the commit did it. Only
// tickets in projectID move.
func moveFixed(ctx context.Context, q *db.Queries, projectID int64, c Commit) error {
	keys := Fixes(c.Message)
	if len(keys) == 0 {
		return nil
	}
	moved, err := q.MoveFixedTickets(ctx, db.MoveFixedTicketsParams{ProjectID: projectID, Keys: keys})
	if err != nil {
		return err
	}
	for _, m := range moved {
		changes, err := json.Marshal(map[string]any{"status": map[string]string{"old": m.OldStatus, "new": m.NewStatus}, "commit": c.SHA})
		if err != nil {
			return err
		}
		if err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{Via: "webhook", Entity: "ticket", EntityID: m.ID, ProjectID: &m.ProjectID,
			Action: "transition", Changes: changes}); err != nil {
			return err
		}
	}
	return nil
}
