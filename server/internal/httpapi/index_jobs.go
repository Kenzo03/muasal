package httpapi

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kenzo03/zettra/server/internal/indexer"
)

// index queues one index job per ticket in the change's transaction, so the
// job commits or rolls back with the change (FSD §4.2, §13.2).
func (s *Server) index(ctx context.Context, tx pgx.Tx, ticketIDs ...int64) error {
	if len(ticketIDs) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(ticketIDs))
	for i, id := range ticketIDs {
		params[i] = river.InsertManyParams{Args: indexer.IndexTicket{TicketID: id}}
	}
	_, err := s.jobs.InsertManyTx(ctx, tx, params)
	return err
}
