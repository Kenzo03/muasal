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
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
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
//
//	Tickets: HRIS-1 core, HRIS-2 Client A (closed), HRIS-3 Client B (closed,
//	with a file), HRIS-4 Client C, PAY-1 Client C. HRIS-2 is on Overtime
//	Approval, PAY-1 on Run Payroll, the others on Leave Request.

var suiteUsers = []string{"admin", "hana", "ani", "budi", "citra", "dodi"}

type world struct {
	as         map[string]*http.Client
	users      map[string]db.User
	clients    map[string]db.Client
	nodes      map[string]db.Node
	contacts   map[string]int64
	fileB      int64 // an attachment of HRIS-3
	inProgress int64 // HRIS's "In progress" status
	done       int64 // HRIS's "Done" status
}

func seedWorld(e *env) world {
	w := world{as: map[string]*http.Client{}, users: map[string]db.User{}, clients: map[string]db.Client{},
		nodes: map[string]db.Node{}, contacts: map[string]int64{}}
	for _, name := range []string{"A", "B", "C"} {
		w.clients[name] = e.seedClient("Client " + name)
	}
	a, b, c := w.clients["A"], w.clients["B"], w.clients["C"]
	hris := e.seedProject("HRIS", a, b, c)
	pay := e.seedProject("PAY", a, c)
	for _, name := range suiteUsers {
		w.as[name], w.users[name] = e.signedIn(name+"@example.com", name == "admin")
	}
	u := w.users
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

	e.seedTicket(hris, u["hana"], "Core fix", nil, w.nodes["Leave Request"])
	closedA := e.seedTicket(hris, u["hana"], "Client A request", &a, w.nodes["Overtime Approval"])
	closedB := e.seedTicket(hris, u["hana"], "Client B request", &b, w.nodes["Leave Request"])
	e.seedTicket(hris, u["hana"], "Client C request", &c, w.nodes["Leave Request"])
	e.seedTicket(pay, u["dodi"], "PAY Client C request", &c, w.nodes["Run Payroll"])
	_, file, _ := upload(e, w.as["hana"], "HRIS-3", "b.txt", []byte("for Client B"))
	w.fileB = file.Id
	statuses, err := e.q.ListStatuses(context.Background(), hris.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	w.inProgress, w.done = statuses[1].ID, statuses[3].ID
	e.seedClose(closedA, w.done, u["hana"], "Client A approves overtime in HR")
	e.seedClose(closedB, w.done, u["hana"], "Client B exports leave balances")
	e.seedNote(hris, u["hana"], "Client A overtime meeting", &a, w.nodes["Overtime Approval"]) // HRIS-DN1
	e.seedNote(hris, u["hana"], "Leave carry-over call", nil, w.nodes["Leave Request"])        // HRIS-DN2
	e.seedNote(hris, u["hana"], "Client B leave email", &b, w.nodes["Leave Request"])          // HRIS-DN3
	return w
}

// listed returns the sorted keys (projects, tickets) or names of a list response.
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
	hrisTickets := []string{"HRIS-1", "HRIS-2", "HRIS-3", "HRIS-4"}
	assignees := []string{"ani@example.com", "budi@example.com", "hana@example.com"}
	statuses := []string{"Cancelled", "Done", "In progress", "In review", "To do"}
	nodePath := func(name, rest string) string { return fmt.Sprintf("/nodes/%d%s", w.nodes[name].ID, rest) }
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
		{"/projects/HRIS/tickets", map[string][]string{
			"admin": hrisTickets, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4"},
		}, map[string]int{"dodi": 404}},
		{"/projects/PAY/tickets", map[string][]string{"admin": {"PAY-1"}, "citra": {"PAY-1"}, "dodi": {"PAY-1"}},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{"/projects/HRIS/assignees", map[string][]string{
			"admin": assignees, "hana": assignees, "ani": assignees, "budi": assignees, "citra": assignees,
		}, map[string]int{"dodi": 404}},
		{"/projects/HRIS/statuses", map[string][]string{
			"admin": statuses, "hana": statuses, "ani": statuses, "budi": statuses, "citra": statuses,
		}, map[string]int{"dodi": 404}},
		{nodePath("Attendance", "/timeline"), map[string][]string{
			"admin": hrisTickets, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4"},
		}, map[string]int{"dodi": 404}},
		{nodePath("Overtime Approval", "/timeline"), map[string][]string{
			"admin": {"HRIS-2"}, "hana": {"HRIS-2"}, "ani": {"HRIS-2"}, "citra": {"HRIS-2"},
		}, map[string]int{"budi": 404, "dodi": 404}},
		{nodePath("Run Payroll", "/timeline"), map[string][]string{"admin": {"PAY-1"}, "citra": {"PAY-1"}, "dodi": {"PAY-1"}},
			map[string]int{"hana": 404, "ani": 404, "budi": 404}},
		{nodePath("HR", "/behaviors"), map[string][]string{
			"admin": {"HRIS-2", "HRIS-3"}, "hana": {"HRIS-2", "HRIS-3"}, "ani": {"HRIS-2", "HRIS-3"},
			"budi": {"HRIS-3"}, "citra": {"HRIS-2"},
		}, map[string]int{"dodi": 404}},
		{"/projects/HRIS/notes", map[string][]string{
			"admin": {"HRIS-DN1", "HRIS-DN2", "HRIS-DN3"}, "hana": {"HRIS-DN1", "HRIS-DN2", "HRIS-DN3"}, "ani": {"HRIS-DN1", "HRIS-DN2", "HRIS-DN3"},
			"budi": {"HRIS-DN2", "HRIS-DN3"}, "citra": {"HRIS-DN1", "HRIS-DN2"},
		}, map[string]int{"dodi": 404}},
		{"/me/updates", map[string][]string{
			"admin": {"HRIS-1", "HRIS-2", "HRIS-3", "HRIS-4", "PAY-1"}, "hana": hrisTickets, "ani": hrisTickets,
			"budi": {"HRIS-1", "HRIS-3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4", "PAY-1"}, "dodi": {"PAY-1"},
		}, nil},
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
	// Single things: the users who may open them get 200, everyone else 404.
	for path, see := range map[string][]string{
		"/projects/HRIS":                        {"admin", "hana", "ani", "budi", "citra"},
		"/tickets/HRIS-2":                       {"admin", "hana", "ani", "citra"},
		"/tickets/HRIS-2/activity":              {"admin", "hana", "ani", "citra"},
		"/tickets/HRIS-3":                       {"admin", "hana", "ani", "budi"},
		fmt.Sprintf("/attachments/%d", w.fileB): {"admin", "hana", "ani", "budi"},
		nodePath("Overtime Approval", ""):       {"admin", "hana", "ani", "citra"},
		"/notes/HRIS-DN1":                       {"admin", "hana", "ani", "citra"},
		"/notes/HRIS-DN3":                       {"admin", "hana", "ani", "budi"},
	} {
		for _, user := range suiteUsers {
			want := http.StatusNotFound
			if slices.Contains(see, user) {
				want = http.StatusOK
			}
			if code := e.call(w.as[user], http.MethodGet, path, nil, nil); code != want {
				t.Errorf("GET %s as %s: %d, want %d", path, user, code, want)
			}
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
	// R-AC-7 in search: results hold only what each user may open, across projects.
	for _, c := range []struct {
		q     string
		nodes bool
		see   map[string][]string
	}{
		{"request", false, map[string][]string{
			"admin": {"HRIS-2", "HRIS-3", "HRIS-4", "PAY-1"}, "hana": {"HRIS-2", "HRIS-3", "HRIS-4"}, "ani": {"HRIS-2", "HRIS-3", "HRIS-4"},
			"budi": {"HRIS-3"}, "citra": {"HRIS-2", "HRIS-4", "PAY-1"}, "dodi": {"PAY-1"},
		}},
		{"overtime", true, map[string][]string{
			"admin": {"Overtime Approval"}, "hana": {"Overtime Approval"}, "ani": {"Overtime Approval"}, "citra": {"Overtime Approval"},
		}},
		{"payroll", true, map[string][]string{
			"admin": {"Payroll", "Run Payroll"}, "citra": {"Payroll", "Run Payroll"}, "dodi": {"Payroll", "Run Payroll"},
		}},
	} {
		for _, user := range suiteUsers {
			var res httpapi.SearchResults
			code := e.call(w.as[user], http.MethodGet, "/search?q="+c.q, nil, &res)
			got := []string{}
			if c.nodes {
				for _, n := range res.Nodes {
					got = append(got, n.Name)
				}
			} else {
				for _, tk := range res.Tickets {
					got = append(got, tk.Key)
				}
			}
			slices.Sort(got)
			if want := c.see[user]; code != http.StatusOK || !slices.Equal(got, want) {
				t.Errorf("search %q as %s: %d %v, want %v", c.q, user, code, got, want)
			}
		}
	}

	// R-AC-7 on Home: My tickets hold only open tickets the assignee may see.
	// HRIS-4 is Client C's, which budi cannot see; HRIS-3 is closed.
	for key, user := range map[string]string{"HRIS-1": "budi", "HRIS-3": "budi", "HRIS-4": "budi", "PAY-1": "citra"} {
		if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET assignee_id = $1 WHERE key = $2", w.users[user].ID, key); err != nil {
			t.Fatal(err)
		}
	}
	for user, want := range map[string][]string{"budi": {"HRIS-1"}, "citra": {"PAY-1"}, "hana": {}} {
		if got, code := listed(e, w.as[user], "/me/tickets"); code != http.StatusOK || !slices.Equal(got, want) {
			t.Errorf("my tickets as %s: %d %v, want %v", user, code, got, want)
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
	coreTicket := map[string]any{"title": "Core clean-up", "type": "bug", "node_ids": []int64{}}
	aTicket := map[string]any{"title": "Client A change", "type": "bug", "node_ids": []int64{}, "client_id": w.clients["A"].ID}
	edit := map[string]any{"title": "Taken over", "type": "bug", "node_ids": []int64{}, "requester_user_id": w.users["budi"].ID}
	ifMatch := map[string]string{"If-Match": `"1"`}
	decision := map[string]any{"what_changed": "Client A approves overtime in HR.", "why": "Supervisors are often on leave."}
	closeCore := map[string]any{"status_id": w.done, "reason": "Every client asked for the core fix.",
		"decision": map[string]any{"what_changed": "The core fix ships to every client.", "why": "Every client asked for the core fix."}}
	for _, c := range []struct {
		user, method, path string
		headers            map[string]string
		body               any
		want               int
	}{
		{"hana", http.MethodPost, "/projects", nil, map[string]any{"key": "NEW", "name": "New"}, 403},
		{"ani", http.MethodPatch, "/projects/HRIS", nil, map[string]any{"name": "X"}, 403},
		{"dodi", http.MethodPatch, "/projects/HRIS", nil, map[string]any{"name": "X"}, 404},
		{"ani", http.MethodPut, "/projects/HRIS/clients", nil, map[string]any{"client_ids": []int64{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/members", nil, map[string]any{"members": []any{}}, 403},
		{"ani", http.MethodPut, "/projects/HRIS/statuses", nil, map[string]any{"statuses": []any{}}, 403},
		{"hana", http.MethodPatch, client("A"), nil, map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/clients", nil, map[string]any{"name": "X"}, 403},
		{"ani", http.MethodPost, "/projects/HRIS/nodes", nil, menu, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/nodes", nil, menu, 404},
		{"ani", http.MethodPatch, node("Leave Request"), nil, map[string]any{"name": "X"}, 403},
		{"budi", http.MethodPatch, node("Overtime Approval"), nil, map[string]any{"name": "X"}, 404}, // hidden by scope
		{"budi", http.MethodDelete, node("OT Rules"), nil, nil, 404},                                 // under a hidden menu
		{"dodi", http.MethodDelete, node("Leave Request"), nil, nil, 404},
		{"budi", http.MethodPatch, contact("Andi"), nil, map[string]any{"name": "X"}, 404},
		{"citra", http.MethodPatch, contact("Andi"), nil, map[string]any{"name": "X"}, 403}, // a viewer for Client A
		{"budi", http.MethodPost, "/contacts", nil, map[string]any{"name": "X", "client_id": w.clients["A"].ID}, 422},
		{"dodi", http.MethodPost, "/contacts", nil, map[string]any{"name": "X", "client_id": w.clients["B"].ID}, 422}, // B is no PAY client
		{"budi", http.MethodPost, "/projects/HRIS/tickets", nil, aTicket, 422},                                        // Client A is outside budi's scope
		{"citra", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 403},
		{"dodi", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 404},
		{"budi", http.MethodPut, "/tickets/HRIS-2", ifMatch, edit, 404},
		{"citra", http.MethodPost, "/tickets/HRIS-2/transition", nil, map[string]any{"status_id": w.inProgress}, 403},
		{"dodi", http.MethodPost, "/tickets/HRIS-1/transition", nil, map[string]any{"status_id": w.inProgress}, 404},
		{"citra", http.MethodPost, "/tickets/HRIS-2/comments", nil, map[string]any{"body": "Seen"}, 403},
		{"budi", http.MethodPost, "/tickets/HRIS-2/comments", nil, map[string]any{"body": "Hi"}, 404},
		{"citra", http.MethodDelete, fmt.Sprintf("/attachments/%d", w.fileB), nil, nil, 404},
		{"budi", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 404},
		{"citra", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 403},
		{"ani", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 403}, // neither project admin nor confirmer
		{"citra", http.MethodPost, "/tickets/HRIS-4/transition", nil, closeCore, 403},
		// Allowed, as controls: the suite must not pass by refusing everything.
		{"hana", http.MethodPatch, node("Leave Request"), nil, map[string]any{"name": "Leave Requests"}, 200},
		{"citra", http.MethodPatch, contact("Cahya"), nil, map[string]any{"name": "Cahya", "client_id": w.clients["C"].ID}, 200},
		{"budi", http.MethodPost, "/projects/HRIS/tickets", nil, coreTicket, 201},
		{"ani", http.MethodPost, "/tickets/HRIS-3/comments", nil, map[string]any{"body": "Checked"}, 201},
		{"budi", http.MethodPost, "/tickets/HRIS-3/transition", nil, map[string]any{"status_id": w.inProgress}, 200},
		{"hana", http.MethodPut, "/tickets/HRIS-2/decision", nil, decision, 200},
		{"budi", http.MethodPost, "/tickets/HRIS-1/transition", nil, closeCore, 200},
	} {
		if code, _ := e.callWith(w.as[c.user], c.method, c.path, c.headers, c.body, nil); code != c.want {
			t.Errorf("%s %s as %s: %d, want %d", c.method, c.path, c.user, code, c.want)
		}
	}
}

// R-AC-7 and AC-AK-3 in Ask: every user's evidence is exactly the tickets they
// may open, and no claim cites anything else, across projects.
func TestPermissionSuiteAsk(t *testing.T) {
	e := newEnv(t)
	w := seedWorld(e)
	fake := llmtest.New(t)
	fake.Answer = func(_, _ string, schema json.RawMessage) string {
		keys := evidenceKeys(schema)
		b, _ := json.Marshal(map[string]any{"claims": []map[string]any{{"text": "Every request changed something.", "cites": keys[:min(4, len(keys))]}}})
		return string(b)
	}
	if code := e.call(w.as["admin"], http.MethodPut, "/admin/settings/ai", aiUpdate("local", fake.BaseURL()), nil); code != http.StatusOK {
		t.Fatalf("switch to Local: %d", code)
	}
	var ids []int64
	rows, err := e.d.Pool.Query(context.Background(), "SELECT id FROM tickets")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	e.indexNow(ids...)

	// Decision notes are evidence under the same predicate (§9.4).
	hris := []string{"HRIS-1", "HRIS-2", "HRIS-3", "HRIS-4", "HRIS-DN1", "HRIS-DN2", "HRIS-DN3"}
	for user, want := range map[string][]string{
		"admin": append(slices.Clone(hris), "PAY-1"), "hana": hris, "ani": hris,
		"budi": {"HRIS-1", "HRIS-3", "HRIS-DN2", "HRIS-DN3"}, "citra": {"HRIS-1", "HRIS-2", "HRIS-4", "HRIS-DN1", "HRIS-DN2", "PAY-1"}, "dodi": {"PAY-1"},
	} {
		var res httpapi.AskResult
		if code := e.call(w.as[user], http.MethodPost, "/ask", map[string]any{"question": "Which request or fix changed what, for Client A, Client B and Client C?"}, &res); code != http.StatusOK {
			t.Fatalf("ask as %s: %d", user, code)
		}
		var got []string
		for _, it := range res.Evidence {
			got = append(got, it.Key)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("evidence as %s: %v, want %v", user, got, want)
		}
		for _, c := range res.Claims {
			for _, k := range c.Cites {
				if !slices.Contains(want, k) {
					t.Errorf("a claim as %s cites %s", user, k)
				}
			}
		}
	}
}

// evidenceKeys reads the citation enum of an answer's schema.
func evidenceKeys(schema json.RawMessage) []string {
	var s struct {
		Properties struct {
			Claims struct {
				Items struct {
					Properties struct {
						Cites struct {
							Items struct {
								Enum []string `json:"enum"`
							} `json:"items"`
						} `json:"cites"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"claims"`
		} `json:"properties"`
	}
	_ = json.Unmarshal(schema, &s)
	return s.Properties.Claims.Items.Properties.Cites.Items.Enum
}
