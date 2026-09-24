package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// The permission suite (FSD §5.3, §21.1) seeds two projects, three clients and
// four scoped users, calls every endpoint as every user and fails on any row
// outside the user's scope, or missing from it. CI runs it on every change, so
// it blocks merges. New endpoints add rows to the tables below.
//
//	HRIS (clients A, B, C)                 PAY (clients A, C)
//	  hana   project admin                   citra  member, Client C
//	  ani    member, all clients             dodi   member, all clients
//	  budi   member, Client B
//	  citra  viewer, Clients A and C
//	plus admin, a system admin who belongs to no project.

var suiteUsers = []string{"admin", "hana", "ani", "budi", "citra", "dodi"}

type world struct {
	as       map[string]*http.Client
	clients  map[string]db.Client
	nodes    map[string]db.Node
	contacts map[string]int64
}

func seedWorld(e *env) world {
	w := world{as: map[string]*http.Client{}, clients: map[string]db.Client{}, nodes: map[string]db.Node{}, contacts: map[string]int64{}}
	for _, name := range []string{"A", "B", "C"} {
		w.clients[name] = e.seedClient("Client " + name)
	}
	a, b, c := w.clients["A"], w.clients["B"], w.clients["C"]
	hris := e.seedProject("HRIS", a, b, c)
	pay := e.seedProject("PAY", a, c)
	u := map[string]db.User{}
	for _, name := range suiteUsers {
		w.as[name], u[name] = e.signedIn(name+"@example.com", name == "admin")
	}
	e.seedMember(u["hana"], hris, "admin")
	e.seedMember(u["ani"], hris, "member")
	e.seedMember(u["budi"], hris, "member", b)
	e.seedMember(u["citra"], hris, "viewer", a, c)
	e.seedMember(u["citra"], pay, "member", c)
	e.seedMember(u["dodi"], pay, "member")

	node := func(p db.Project, parent, typ, name string, clients ...db.Client) {
		var up *db.Node
		if parent != "" {
			n := w.nodes[parent]
			up = &n
		}
		w.nodes[name] = e.seedNode(p, up, typ, name, clients...)
	}
	node(hris, "", "module", "HR")
	node(hris, "HR", "module", "Attendance")
	node(hris, "Attendance", "menu", "Overtime Approval", a)
	node(hris, "Overtime Approval", "menu", "OT Rules")
	node(hris, "Attendance", "menu", "Leave Request")
	node(hris, "HR", "menu", "Payslip Export", b, c)
	node(hris, "", "module", "Core")
	node(pay, "", "module", "Payroll")
	node(pay, "Payroll", "menu", "Run Payroll", c)

	w.contacts["Dewi"] = e.seedContact("Dewi", nil) // internal
	w.contacts["Andi"] = e.seedContact("Andi", &a)
	w.contacts["Bayu"] = e.seedContact("Bayu", &b)
	w.contacts["Cahya"] = e.seedContact("Cahya", &c)
	return w
}

// listed returns the sorted keys (projects) or names of a list response.
func listed(e *env, c *http.Client, path string) ([]string, int) {
	var list struct {
		Items []struct{ Key, Name string } `json:"items"`
	}
	code := e.call(c, http.MethodGet, path, nil, &list)
	out := []string{}
	for _, it := range list.Items {
		if it.Key != "" {
			out = append(out, it.Key)
		} else {
			out = append(out, it.Name)
		}
	}
	slices.Sort(out)
	return out, code
}

func TestPermissionSuiteReads(t *testing.T) {
	e := newEnv(t)
	w := seedWorld(e)
	hrisTree := []string{"Attendance", "Core", "HR", "Leave Request", "OT Rules", "Overtime Approval", "Payslip Export"}
	payTree := []string{"Payroll", "Run Payroll"}
	allContacts := []string{"Andi", "Bayu", "Cahya", "Dewi"}
	abc := []string{"Client A", "Client B", "Client C"}
	hrisMembers := []string{"ani@example.com", "budi@example.com", "citra@example.com", "hana@example.com"}
	for _, c := range []struct {
		path string
		see  map[string][]string // these users get exactly these rows
		deny map[string]int      // the others get this status
	}{
		{"/projects", map[string][]string{
			"admin": {"HRIS", "PAY"}, "hana": {"HRIS"}, "ani": {"HRIS"}, "budi": {"HRIS"}, "citra": {"HRIS", "PAY"}, "dodi": {"PAY"},
		}, nil},
		{"/projects/HRIS/nodes", map[string][]string{
			"admin": hrisTree, "hana": hrisTree, "ani": hrisTree, "citra": hrisTree,
			"budi": {"Attendance", "Core", "HR", "Leave Request", "Payslip Export"},
		}, map[string]int{"dodi": 404}},
		{"/projects/PAY/nodes", map[string][]string{"admin": payTree, "citra": payTree, "dodi": payTree},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{"/contacts", map[string][]string{
			"admin": allContacts, "hana": allContacts, "ani": allContacts,
			"budi": {"Bayu", "Dewi"}, "citra": {"Andi", "Cahya", "Dewi"}, "dodi": {"Andi", "Cahya", "Dewi"},
		}, nil},
		{"/clients", map[string][]string{"admin": abc, "hana": abc},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 403}},
		{"/projects/HRIS/clients", map[string][]string{
			"admin": abc, "hana": abc, "ani": abc, "budi": {"Client B"}, "citra": {"Client A", "Client C"},
		}, map[string]int{"dodi": 404}},
		{"/projects/HRIS/members", map[string][]string{"admin": hrisMembers, "hana": hrisMembers},
			map[string]int{"ani": 403, "budi": 403, "citra": 403, "dodi": 404}},
	} {
		for _, user := range suiteUsers {
			got, code := listed(e, w.as[user], c.path)
			if want, ok := c.see[user]; ok {
				if code != http.StatusOK || !slices.Equal(got, want) {
					t.Errorf("GET %s as %s: %d %v, want %v", c.path, user, code, got, want)
				}
			} else if code != c.deny[user] {
				t.Errorf("GET %s as %s: %d, want %d", c.path, user, code, c.deny[user])
			}
		}
	}
	for _, user := range suiteUsers {
		want := http.StatusOK
		if user == "dodi" {
			want = http.StatusNotFound
		}
		if code := e.call(w.as[user], http.MethodGet, "/projects/HRIS", nil, nil); code != want {
			t.Errorf("GET /projects/HRIS as %s: %d, want %d", user, code, want)
		}
	}
	// R-MR-8: a client-specific menu names only in-scope clients.
	for user, want := range map[string][]string{"ani": {"Client B", "Client C"}, "budi": {"Client B"}, "citra": {"Client C"}} {
		var list httpapi.NodeList
		e.call(w.as[user], http.MethodGet, "/projects/HRIS/nodes", nil, &list)
		for _, n := range list.Items {
			if n.Name != "Payslip Export" {
				continue
			}
			var got []string
			for _, c := range n.Clients {
				got = append(got, c.Name)
			}
			if !slices.Equal(got, want) {
				t.Errorf("Payslip Export clients as %s: %v, want %v", user, got, want)
			}
		}
	}
}

func TestPermissionSuiteWrites(t *testing.T) {
	e := newEnv(t)
	w := seedWorld(e)
	node := func(name string) string { return fmt.Sprintf("/nodes/%d", w.nodes[name].ID) }
	contact := func(name string) string { return fmt.Sprintf("/contacts/%d", w.contacts[name]) }
	client := func(name string) string { return fmt.Sprintf("/clients/%d", w.clients[name].ID) }
	menu := map[string]any{"type": "menu", "name": "X"}
	for _, c := range []struct {
		user, method, path string
		body               any
		want               int
	}{
		{"hana", http.MethodPost, "/projects", map[string]any{"key": "NEW", "name": "New"}, 403},
		{"ani", http.MethodPatch, "/projects/HRIS", map[string]any{"name": "X"}, 403},
		{"dodi", http.MethodPatch, "/projects/HRIS", map[string]any{"name": "X"}, 404},
		{"ani", http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/members", map[string]any{"members": []any{}}, 403},
		{"hana", http.MethodPatch, client("A"), map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/clients", map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/projects/HRIS/nodes", menu, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/nodes", menu, 404},
		{"ani", http.MethodPatch, node("Leave Request"), map[string]any{"name": "X"}, 403},
		{"budi", http.MethodPatch, node("Overtime Approval"), map[string]any{"name": "X"}, 404}, // hidden by scope
		{"budi", http.MethodDelete, node("OT Rules"), nil, 404},                                 // under a hidden menu
		{"dodi", http.MethodDelete, node("Leave Request"), nil, 404},
		{"budi", http.MethodPatch, contact("Andi"), map[string]any{"name": "X"}, 404},
		{"citra", http.MethodPatch, contact("Andi"), map[string]any{"name": "X"}, 403}, // a viewer for Client A
		{"budi", http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": w.clients["A"].ID}, 422},
		{"dodi", http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": w.clients["B"].ID}, 422}, // B is no PAY client
		// Allowed, as controls: the suite must not pass by refusing everything.
		{"hana", http.MethodPatch, node("Leave Request"), map[string]any{"name": "Leave Requests"}, 200},
		{"citra", http.MethodPatch, contact("Cahya"), map[string]any{"name": "Cahya", "client_id": w.clients["C"].ID}, 200},
	} {
		if code := e.call(w.as[c.user], c.method, c.path, c.body, nil); code != c.want {
			t.Errorf("%s %s as %s: %d, want %d", c.method, c.path, c.user, code, c.want)
		}
	}
}
