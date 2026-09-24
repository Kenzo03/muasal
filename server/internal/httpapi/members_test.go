package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestProjectAdminSetsMembersAndScopes(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedUser("budi@example.com", pw, false)
	e.seedUser("ani@example.com", pw, false)
	body := map[string]any{"members": []map[string]any{
		{"email": "owner@example.com", "role": "admin", "all_clients": true},
		{"email": "BUDI@example.com", "role": "member", "all_clients": false, "client_ids": []int64{b.ID}},
		{"email": "ani@example.com", "role": "viewer", "all_clients": true, "client_ids": []int64{a.ID}},
	}}
	var list httpapi.MemberList
	if code := e.call(owner, http.MethodPut, "/projects/HRIS/members", body, &list); code != http.StatusOK || len(list.Items) != 3 {
		t.Fatalf("set: %d %+v", code, list)
	}
	byEmail := map[string]httpapi.Member{}
	for _, m := range list.Items {
		byEmail[m.Email] = m
	}
	if m := byEmail["budi@example.com"]; m.Role != httpapi.ProjectRoleMember || m.AllClients || !slices.Equal(m.ClientIds, []int64{b.ID}) {
		t.Fatalf("budi: %+v", m)
	}
	if m := byEmail["ani@example.com"]; !m.AllClients || len(m.ClientIds) != 0 {
		t.Fatalf("all clients ignores the list: %+v", m)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "project", EntityID: p.ID})
	if err != nil || len(events) != 1 || events[0].Action != "set_members" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestMemberUpdatesAreValidated(t *testing.T) {
	e := newEnv(t)
	a, other := e.seedClient("Client A"), e.seedClient("Client Z")
	p := e.seedProject("HRIS", a)
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedUser("budi@example.com", pw, false)
	self := map[string]any{"email": "owner@example.com", "role": "admin", "all_clients": true}
	for _, c := range []struct {
		members     []map[string]any
		field, code string
	}{
		{[]map[string]any{self, {"email": "budi@example.com", "role": "admin", "all_clients": false}}, "members[1].all_clients", "admin_needs_all_clients"},
		{[]map[string]any{self, {"email": "nobody@example.com", "role": "member", "all_clients": true}}, "members[1].email", "unknown_user"},
		{[]map[string]any{self, self}, "members[1].email", "duplicate"},
		{[]map[string]any{self, {"email": "budi@example.com", "role": "owner", "all_clients": true}}, "members[1].role", "invalid"},
		{[]map[string]any{{"email": "owner@example.com", "role": "member", "all_clients": true}}, "members", "cannot_demote_self"},
		{[]map[string]any{self, {"email": "budi@example.com", "role": "member", "all_clients": false, "client_ids": []int64{other.ID}}}, "members", "client_not_linked"},
	} {
		var prob httpapi.Problem
		code := e.call(owner, http.MethodPut, "/projects/HRIS/members", map[string]any{"members": c.members}, &prob)
		if f := firstError(prob); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.code, code, prob)
		}
	}
}

// R-AC-8: a changed scope applies on the user's next request.
func TestAScopeChangeAppliesOnTheNextRequest(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member")
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusOK {
		t.Fatalf("before: %d", code)
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/members", map[string]any{"members": []any{}}, nil); code != http.StatusOK {
		t.Fatalf("remove everyone: %d", code)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusNotFound {
		t.Fatalf("after: %d", code)
	}
}
