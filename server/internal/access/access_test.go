package access_test

import (
	"context"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/testdb"
)

func TestRolesRankViewerMemberAdmin(t *testing.T) {
	for _, c := range []struct {
		role, need string
		ok         bool
	}{
		{access.Viewer, access.Viewer, true},
		{access.Viewer, access.Member, false},
		{access.Member, access.Member, true},
		{access.Member, access.Admin, false},
		{access.Admin, access.Viewer, true},
		{"", access.Viewer, false},
	} {
		if got := (access.Scope{Role: c.role}).Allows(c.need); got != c.ok {
			t.Errorf("%q allows %q = %v, want %v", c.role, c.need, got, c.ok)
		}
	}
}

func TestForProjectReadsTheMembership(t *testing.T) {
	d := testdb.New(t)
	q := db.New(d.Pool)
	ctx := context.Background()
	user := func(email string, admin bool) db.User {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: email, IsAdmin: admin, Locale: "id", Timezone: "Asia/Jakarta"})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	p, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := q.CreateClient(ctx, db.CreateClientParams{Name: "Client B"})
	if err != nil {
		t.Fatal(err)
	}
	admin, all, scoped, outsider := user("admin@example.com", true), user("all@example.com", false),
		user("scoped@example.com", false), user("out@example.com", false)
	for _, err := range []error{
		q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: []int64{b.ID}}),
		q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: all.ID, ProjectID: p.ID, Role: access.Member, AllClients: true}),
		q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: scoped.ID, ProjectID: p.ID, Role: access.Viewer}),
		q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: scoped.ID, ProjectID: p.ID, ClientIds: []int64{b.ID}}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name   string
		u      db.User
		want   access.Scope
		member bool
	}{
		{"system admin", admin, access.Scope{Role: access.Admin, AllClients: true}, true},
		{"all clients", all, access.Scope{Role: access.Member, AllClients: true}, true},
		{"scoped", scoped, access.Scope{Role: access.Viewer, ClientIDs: []int64{b.ID}}, true},
		{"outsider", outsider, access.Scope{}, false},
	} {
		got, member, err := access.ForProject(ctx, q, &c.u, p.ID)
		if err != nil || member != c.member || got.Role != c.want.Role || got.AllClients != c.want.AllClients ||
			!slices.Equal(got.ClientIDs, c.want.ClientIDs) {
			t.Errorf("%s: %+v member=%v err=%v", c.name, got, member, err)
		}
	}
	if ok, err := access.AdminsAnything(ctx, q, &all); err != nil || ok {
		t.Errorf("a member is no admin: %v %v", ok, err)
	}
}

func TestScopeSeesItsClientsAndCoreWork(t *testing.T) {
	a, b := int64(1), int64(2)
	scoped := access.Scope{Role: access.Member, ClientIDs: []int64{a}}
	all := access.Scope{Role: access.Member, AllClients: true}
	for _, c := range []struct {
		scope  access.Scope
		client *int64
		sees   bool
	}{
		{scoped, nil, true}, {scoped, &a, true}, {scoped, &b, false}, {all, &b, true},
	} {
		if got := c.scope.Sees(c.client); got != c.sees {
			t.Errorf("%+v sees %v: %v, want %v", c.scope, c.client, got, c.sees)
		}
	}
}
