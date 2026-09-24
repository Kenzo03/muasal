package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func ptr[T any](v T) *T { return &v }

// The handlers turn these constraint names into API problems, so the names are
// part of the contract: each case must fail on exactly the named constraint.
func TestRegistryRulesAreEnforcedByTheDatabase(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u := must(q.CreateUser(ctx, db.CreateUserParams{Email: "u@example.com", Name: "U", Locale: "id", Timezone: "Asia/Jakarta"}))
	p := must(q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	a := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client A"}))
	b := must(q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"}))
	check(q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	check(q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: "member"}))
	check(q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: []int64{a.ID}}))
	hr := must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "HR"}))
	must(q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, ParentID: &hr.ID, Type: "menu", Name: "Leave", Code: ptr("HR.LV")}))

	for _, c := range []struct {
		constraint string
		op         func(q *db.Queries) error
	}{
		{"projects_key_format", func(q *db.Queries) error {
			_, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "hris2", Name: "x"})
			return err
		}},
		{"memberships_admin_all_clients", func(q *db.Queries) error {
			return q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: "admin"})
		}},
		{"membership_clients_linked", func(q *db.Queries) error { // B is not linked to HRIS
			return q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}})
		}},
		{"membership_clients_linked", func(q *db.Queries) error { // A is still in a member scope
			return q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: p.ID, ClientIds: []int64{}})
		}},
		{"project_clients_client_fk", func(q *db.Queries) error {
			return q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{999999}})
		}},
		{"nodes_specific_menu", func(q *db.Queries) error {
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "Payroll", ClientSpecific: true})
			return err
		}},
		{"node_clients_linked", func(q *db.Queries) error {
			return q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: hr.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}})
		}},
		{"nodes_sibling_uq", func(q *db.Queries) error { // names are unique among siblings, ignoring case
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "hr"})
			return err
		}},
		{"nodes_code_uq", func(q *db.Queries) error {
			_, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "menu", Name: "Other", Code: ptr("HR.LV")})
			return err
		}},
		{"nodes_parent_fk", func(q *db.Queries) error { return q.DeleteNode(ctx, hr.ID) }},
		{"contacts_client_fk", func(q *db.Queries) error {
			_, err := q.CreateContact(ctx, db.CreateContactParams{ClientID: ptr(int64(999999)), Name: "Ghost"})
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
