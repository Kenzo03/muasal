package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func nodeNames(list httpapi.NodeList) []string {
	names := make([]string, len(list.Items))
	for i, n := range list.Items {
		names[i] = n.Name
	}
	return names
}

// The Iteration 1 exit check at API level (AC-MR-1, AC-MR-2).
func TestAdminBuildsTheTreeAndScopedMembersSeeTheirMenus(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	admin, _ := e.signedIn("admin@example.com", true)
	create := func(parent *httpapi.Node, typ, name string, clients ...int64) httpapi.Node {
		t.Helper()
		body := map[string]any{"type": typ, "name": name}
		if parent != nil {
			body["parent_id"] = parent.Id
		}
		if len(clients) > 0 {
			body["client_specific"] = true
			body["client_ids"] = clients
		}
		var n httpapi.Node
		if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", body, &n); code != http.StatusCreated {
			t.Fatalf("create %s: %d", name, code)
		}
		return n
	}
	hr := create(nil, "module", "HR")
	att := create(&hr, "module", "Attendance")
	ot := create(&att, "menu", "Overtime Approval", a.ID)
	create(&ot, "menu", "OT Rules")
	create(&att, "menu", "Leave Request")
	if ot.ParentId == nil || *ot.ParentId != att.Id || len(ot.Clients) != 1 || ot.Clients[0].Name != "Client A" {
		t.Fatalf("overtime: %+v", ot)
	}

	scopedB, ub := e.signedIn("budi@example.com", false)
	e.seedMember(ub, p, "member", b)
	allClients, ua := e.signedIn("ani@example.com", false)
	e.seedMember(ua, p, "viewer")

	var list httpapi.NodeList
	if code := e.call(scopedB, http.MethodGet, "/projects/HRIS/nodes", nil, &list); code != http.StatusOK {
		t.Fatalf("list: %d", code)
	}
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Attendance", "Leave Request"}) {
		t.Fatalf("scoped to Client B sees %v", got)
	}
	e.call(allClients, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Attendance", "Overtime Approval", "Leave Request", "OT Rules"}) {
		t.Fatalf("all clients sees %v", got)
	}
	if c := list.Items[2].Clients; len(c) != 1 || c[0].Name != "Client A" {
		t.Fatalf("badge: %+v", c)
	}
}

func TestNodeNamesAndCodesAreUnique(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var hr httpapi.Node
	e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "module", "name": "HR", "code": "HR"}, &hr)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "module", "name": "hr"}, &p); code != http.StatusConflict || p.Code != "node_name_taken" {
		t.Fatalf("sibling name: %d %+v", code, p)
	}
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "Payroll", "code": "HR"}, &p); code != http.StatusConflict || p.Code != "node_code_taken" {
		t.Fatalf("code: %d %+v", code, p)
	}
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "HR", "parent_id": hr.Id}, nil); code != http.StatusCreated {
		t.Fatalf("the same name under another parent: %d", code)
	}
}

func TestClientScopeRulesForMenus(t *testing.T) {
	e := newEnv(t)
	a, other := e.seedClient("Client A"), e.seedClient("Client Z")
	e.seedProject("HRIS", a)
	pay := e.seedNode(e.seedProject("PAY"), nil, "module", "Payroll")
	admin, _ := e.signedIn("admin@example.com", true)
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{map[string]any{"type": "module", "name": "M", "client_specific": true, "client_ids": []int64{a.ID}}, "client_specific", "client_specific_module"},
		{map[string]any{"type": "menu", "name": "M", "client_specific": true}, "client_ids", "required"},
		{map[string]any{"type": "menu", "name": "M", "client_specific": true, "client_ids": []int64{other.ID}}, "client_ids", "client_not_linked"},
		{map[string]any{"type": "screen", "name": "M"}, "type", "invalid"},
		{map[string]any{"type": "menu", "name": " "}, "name", "required"},
		{map[string]any{"type": "menu", "name": "M", "parent_id": pay.ID}, "parent_id", "invalid"}, // a parent in another project
	} {
		var p httpapi.Problem
		code := e.call(admin, http.MethodPost, "/projects/HRIS/nodes", c.body, &p)
		if f := firstError(p); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%v: %d %+v", c.body, code, p)
		}
	}
}

func TestEditingANodeRecordsOldAndNewValues(t *testing.T) {
	e := newEnv(t)
	a := e.seedClient("Client A")
	p := e.seedProject("HRIS", a)
	ot := e.seedNode(p, nil, "menu", "Overtime")
	admin, _ := e.signedIn("admin@example.com", true)
	path := fmt.Sprintf("/nodes/%d", ot.ID)
	var n httpapi.Node
	body := map[string]any{"name": "Overtime Approval", "code": "HR.OT", "aliases": []string{" Persetujuan Lembur ", ""},
		"client_specific": true, "client_ids": []int64{a.ID}}
	if code := e.call(admin, http.MethodPatch, path, body, &n); code != http.StatusOK || n.Name != "Overtime Approval" ||
		n.Code == nil || *n.Code != "HR.OT" || !slices.Equal(n.Aliases, []string{"Persetujuan Lembur"}) || len(n.Clients) != 1 {
		t.Fatalf("edit: %d %+v", code, n)
	}
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"type": "module"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "client_specific_module" {
		t.Fatalf("a client-specific menu cannot become a module: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"type": "module", "client_specific": false, "code": ""}, &n); code != http.StatusOK ||
		n.Type != httpapi.NodeTypeModule || n.Code != nil || len(n.Clients) != 0 {
		t.Fatalf("to a shared module: %d %+v", code, n)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "node", EntityID: ot.ID})
	if err != nil || len(events) != 2 {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["name"]["old"] != "Overtime" || changes["name"]["new"] != "Overtime Approval" {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}

func TestMovingAndDeletingNodes(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	hr := e.seedNode(p, nil, "module", "HR")
	att := e.seedNode(p, &hr, "module", "Attendance")
	leave := e.seedNode(p, &att, "menu", "Leave Request")
	e.seedNode(p, &att, "menu", "Overtime")
	admin, _ := e.signedIn("admin@example.com", true)
	patch := func(id int64, body map[string]any, out any) int {
		return e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", id), body, out)
	}
	var moved httpapi.Node
	if code := patch(leave.ID, map[string]any{"move": map[string]any{"parent_id": hr.ID, "position": 0}}, &moved); code != http.StatusOK ||
		moved.ParentId == nil || *moved.ParentId != hr.ID || moved.Position != 0 {
		t.Fatalf("move: %d %+v", code, moved)
	}
	var list httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if got := nodeNames(list); !slices.Equal(got, []string{"HR", "Leave Request", "Attendance", "Overtime"}) {
		t.Fatalf("after the move: %v", got)
	}
	var prob httpapi.Problem
	if code := patch(hr.ID, map[string]any{"move": map[string]any{"parent_id": att.ID}}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "node_cycle" {
		t.Fatalf("cycle: %d %+v", code, prob)
	}
	if code := patch(att.ID, map[string]any{"move": map[string]any{"parent_id": nil}}, &moved); code != http.StatusOK || moved.ParentId != nil {
		t.Fatalf("to the top level: %d %+v", code, moved)
	}
	// R-MR-5: the audit log keeps the old and the new parent.
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "node", EntityID: leave.ID})
	if err != nil || len(events) != 1 {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["parent_id"]["old"] != float64(att.ID) || changes["parent_id"]["new"] != float64(hr.ID) {
		t.Fatalf("changes: %s", events[0].Changes)
	}
	if code := e.call(admin, http.MethodDelete, fmt.Sprintf("/nodes/%d", att.ID), nil, &prob); code != http.StatusConflict || prob.Code != "node_has_children" {
		t.Fatalf("delete a parent: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodDelete, fmt.Sprintf("/nodes/%d", leave.ID), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete a leaf: %d", code)
	}
}

func TestOnlyProjectAdminsChangeTheTree(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	secret := e.seedNode(p, nil, "menu", "Client A Report", a)
	open := e.seedNode(p, nil, "menu", "Leave")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member", b)
	outsider, _ := e.signedIn("out@example.com", false)
	for _, c := range []struct {
		who  *http.Client
		id   int64
		want int
	}{
		{member, open.ID, http.StatusForbidden},  // visible, but members do not edit the tree
		{member, secret.ID, http.StatusNotFound}, // hidden by the client scope, so it looks missing (R-AC-7)
		{outsider, open.ID, http.StatusNotFound},
		{member, 999999, http.StatusNotFound},
	} {
		if code := e.call(c.who, http.MethodPatch, fmt.Sprintf("/nodes/%d", c.id), map[string]any{"name": "X"}, nil); code != c.want {
			t.Errorf("node %d: %d, want %d", c.id, code, c.want)
		}
	}
	if code := e.call(member, http.MethodPost, "/projects/HRIS/nodes", map[string]any{"type": "menu", "name": "X"}, nil); code != http.StatusForbidden {
		t.Errorf("member creates: %d", code)
	}
}
