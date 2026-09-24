package db_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// R-TK-2 and the ticket constraints the handlers turn into API problems.
func TestTicketRulesAreEnforcedByTheDatabase(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	pay := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "PAY", Name: "PAY"}))

	statuses := must(q.ListStatuses(ctx, p.ID))
	var names []string
	for _, s := range statuses {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"To do", "In progress", "In review", "Done", "Cancelled"}) || !statuses[0].IsDefault {
		t.Fatalf("default statuses: %+v", statuses)
	}
	a := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client A"}))
	b := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"}))
	check(q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	node := must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "menu", Name: "Leave"}))
	ticket := func(n int64, mutate func(*db.CreateTicketParams)) db.CreateTicketParams {
		params := db.CreateTicketParams{
			ProjectID: p.ID, Number: n, Key: fmt.Sprintf("HRIS-%d", n), Type: "bug", Title: "A ticket",
			StatusID: statuses[0].ID, RequesterUserID: &u.ID, ReporterID: u.ID, Priority: "medium",
		}
		if mutate != nil {
			mutate(&params)
		}
		return params
	}
	first := must(q.CreateTicket(ctx, ticket(must(q.NextTicketNumber(ctx, p.ID)), func(t *db.CreateTicketParams) { t.ClientID = &a.ID })))
	check(q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: first.ID, NodeIds: []int64{node.ID}}))
	if first.Number != 1 || first.Version != 1 {
		t.Fatalf("first ticket: %+v", first)
	}
	payStatus := must(q.ListStatuses(ctx, pay.ID))[0]

	for _, c := range []struct {
		constraint string
		op         func(q *db.Queries) error
	}{
		{"tickets_status_same_project", func(q *db.Queries) error {
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.StatusID = payStatus.ID }))
			return err
		}},
		{"tickets_client_linked", func(q *db.Queries) error { // B is not a client of HRIS
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.ClientID = &b.ID }))
			return err
		}},
		{"tickets_one_requester", func(q *db.Queries) error {
			_, err := q.CreateTicket(ctx, ticket(2, func(t *db.CreateTicketParams) { t.RequesterUserID = nil }))
			return err
		}},
		{"tickets_client_linked", func(q *db.Queries) error { // A is still used by a ticket
			return q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: p.ID, ClientIds: []int64{}})
		}},
		{"tickets_status_same_project", func(q *db.Queries) error { // "To do" is still used by a ticket
			return q.DeleteStatusesExcept(ctx, db.DeleteStatusesExceptParams{ProjectID: p.ID, KeepIds: []int64{statuses[1].ID}})
		}},
		{"ticket_nodes_node_fk", func(q *db.Queries) error { return q.DeleteNode(ctx, node.ID) }},
		{"statuses_default_uq", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: p.ID, Name: "Backlog", Category: "todo", Position: 9, Color: "#000000", IsDefault: true})
			return err
		}},
		{"statuses_default_todo", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: pay.ID, Name: "Shipped", Category: "done", Position: 9, Color: "#000000", IsDefault: true})
			return err
		}},
		{"statuses_name_uq", func(q *db.Queries) error {
			_, err := q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: p.ID, Name: "to do", Category: "todo", Position: 9, Color: "#000000"})
			return err
		}},
	} {
		tx, err := d.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = c.op(q.WithTx(tx))
		_ = tx.Rollback(ctx)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.ConstraintName != c.constraint {
			t.Errorf("want a %s violation, got %v", c.constraint, err)
		}
	}
}
