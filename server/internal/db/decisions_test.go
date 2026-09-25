package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// R-DC-2, R-DC-4 and R-DC-6 at the database: a close stamps closed_at, a
// stale version moves nothing, a record is confirmed by a user, a reopen turns
// it back into a draft, and the constraints the handlers rely on hold.
func TestDecisionRecordsFollowTheTicket(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	statuses := must(q.ListStatuses(ctx, p.ID))
	todo, done := statuses[0], statuses[3]
	tk := must(q.CreateTicket(ctx, db.CreateTicketParams{
		ProjectID: p.ID, Number: must(q.NextTicketNumber(ctx, p.ID)), Key: "HRIS-1", Type: "bug", Title: "A ticket",
		StatusID: todo.ID, RequesterUserID: &u.ID, ReporterID: u.ID, Priority: "medium",
	}))

	closed := must(q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: done.ID, Closed: true, Version: &tk.Version}))
	if closed.ClosedAt == nil || closed.Version != 2 {
		t.Fatalf("close: %+v", closed)
	}
	if _, err := q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: todo.ID, Version: &tk.Version}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a stale version moved the ticket: %v", err)
	}
	if inUse := must(q.ListStatusIDsInUse(ctx, p.ID)); len(inUse) != 1 || inUse[0] != done.ID {
		t.Fatalf("statuses in use: %v", inUse)
	}

	rec := must(q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
		TicketID: tk.ID, WhatChanged: "Skips the supervisor", Why: "Supervisors are on leave", Outcome: "implemented", ConfirmedBy: u.ID,
	}))
	if rec.State != "confirmed" || rec.ConfirmedBy == nil || *rec.ConfirmedBy != u.ID || rec.ConfirmedAt == nil {
		t.Fatalf("confirmed: %+v", rec)
	}
	check(q.DraftDecision(ctx, tk.ID))
	draft := must(q.GetDecision(ctx, tk.ID))
	if draft.DecisionRecord.State != "draft" || draft.DecisionRecord.ConfirmedBy != nil || draft.DecisionRecord.WhatChanged != "Skips the supervisor" {
		t.Fatalf("draft: %+v", draft)
	}
	again := must(q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
		TicketID: tk.ID, WhatChanged: "Skips the supervisor for Client A", Why: "Supervisors are on leave", Outcome: "rejected", ConfirmedBy: u.ID,
	}))
	if again.Outcome != "rejected" || again.State != "confirmed" || again.WhatChanged != "Skips the supervisor for Client A" {
		t.Fatalf("confirmed again: %+v", again)
	}
	if row := must(q.GetDecision(ctx, tk.ID)); row.ConfirmerName == nil || *row.ConfirmerName != "U" {
		t.Fatalf("confirmer name: %+v", row)
	}

	for _, c := range []struct{ constraint, sql string }{
		{"decision_records_outcome_check", "UPDATE decision_records SET outcome = 'maybe'"},
		{"decision_records_state_check", "UPDATE decision_records SET state = 'final'"},
		{"decision_records_confirmed", "UPDATE decision_records SET confirmed_by = NULL"},
	} {
		_, err := d.Pool.Exec(ctx, c.sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != c.constraint {
			t.Errorf("%s: %v", c.constraint, err)
		}
	}
}
