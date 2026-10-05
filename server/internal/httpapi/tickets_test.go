package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// callWith sends a request with extra headers and returns the status and the response headers.
func (e *env) callWith(c *http.Client, method, path string, headers map[string]string, body, out any) (int, http.Header) {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+"/api/v1"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode, res.Header
}

// hrisWorld is project HRIS with two clients, a small tree and a PM scoped to Client A.
type hrisWorld struct {
	p      db.Project
	a, b   db.Client
	hr, ot db.Node // HR › Overtime Approval, a Client A menu
	secret db.Node // HR › Client B Report, hidden from the PM
	budi   int64   // Budi (HR Manager), a contact of Client A
	pm     *http.Client
	pmUser db.User
}

func newHRIS(e *env) hrisWorld {
	w := hrisWorld{a: e.seedClient("Client A"), b: e.seedClient("Client B")}
	w.p = e.seedProject("HRIS", w.a, w.b)
	w.hr = e.seedNode(w.p, nil, "module", "HR")
	w.ot = e.seedNode(w.p, &w.hr, "menu", "Overtime Approval", w.a)
	w.secret = e.seedNode(w.p, &w.hr, "menu", "Client B Report", w.b)
	title := "HR Manager"
	id, err := e.q.CreateContact(context.Background(), db.CreateContactParams{ClientID: &w.a.ID, Name: "Budi", Title: &title})
	if err != nil {
		e.t.Fatal(err)
	}
	w.budi = id
	w.pm, w.pmUser = e.signedIn("pm@example.com", false)
	e.seedMember(w.pmUser, w.p, "member", w.a)
	return w
}

// The Iteration 2 exit check at API level (story 3, AC-TK-3).
func TestPMLogsAClientRequestInOneRequest(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	reason := "Client A supervisors are often on leave; HR approves overtime directly."
	body := map[string]any{
		"client_id": w.a.ID, "requester_contact_id": w.budi, "title": "Skip supervisor approval for overtime",
		"node_ids": []int64{w.ot.ID}, "reason": reason, "type": "change_request",
	}
	var tk httpapi.Ticket
	code, h := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", nil, body, &tk)
	if code != http.StatusCreated || tk.Key != "HRIS-1" || tk.Status.Name != "To do" || !tk.Status.IsDefault ||
		tk.Client == nil || tk.Client.Name != "Client A" || tk.Requester.Kind != httpapi.TicketRequesterKindContact ||
		tk.Requester.Name != "Budi" || tk.Requester.Title == nil || *tk.Requester.Title != "HR Manager" ||
		len(tk.Nodes) != 1 || tk.Nodes[0].Name != "Overtime Approval" || tk.Reporter.Name != "pm@example.com" ||
		tk.Priority != httpapi.PriorityMedium || h.Get("ETag") != `"1"` {
		t.Fatalf("create: %d %+v %q", code, tk, h.Get("ETag"))
	}
	var got httpapi.Ticket
	if code := e.call(w.pm, http.MethodGet, "/tickets/hris-1", nil, &got); code != http.StatusOK || got.Reason != reason {
		t.Fatalf("read: %d %+v", code, got)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.Id)
	if err != nil || len(events) != 1 || events[0].Action != "create" {
		t.Fatalf("history: %+v %v", events, err)
	}
}

func TestTicketFieldRules(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	bayu := e.seedContact("Bayu", &w.b)
	viewer := e.seedUser("vera@example.com", pw, false)
	e.seedMember(viewer, w.p, "viewer")
	var statuses httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &statuses)
	base := map[string]any{"title": "A proper title", "type": "bug", "node_ids": []int64{}}
	with := func(k string, v any) map[string]any {
		m := map[string]any{k: v}
		for kk, vv := range base {
			if kk != k {
				m[kk] = vv
			}
		}
		return m
	}
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{with("title", "Hi"), "title", "invalid"},
		{with("type", "question"), "type", "invalid"},
		{with("client_id", w.b.ID), "client_id", "invalid"},                       // outside the PM's scope (AC-TK-4)
		{with("requester_contact_id", bayu), "requester_contact_id", "invalid"},   // a contact the PM cannot see
		{with("requester_contact_id", w.budi), "requester_contact_id", "invalid"}, // a Client A contact on core work
		{with("node_ids", []int64{w.secret.ID}), "node_ids", "invalid"},           // a menu hidden from the PM
		{with("assignee_id", viewer.ID), "assignee_id", "invalid"},                // viewers do not own tickets
		{with("status_id", statuses.Items[3].Id), "status_id", "invalid"},         // Done: tickets start open
	} {
		var p httpapi.Problem
		code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", c.body, &p)
		if f := firstError(p); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.field, code, p)
		}
	}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", base, nil); code != http.StatusCreated {
		t.Fatalf("core work by a scoped member (AC-TK-4): %d", code)
	}
}

func TestStaleSavesAreRefused(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	path := "/tickets/" + tk.Key
	edit := map[string]any{"title": "Overtime export for payroll", "type": "feature", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{w.ot.ID}}
	var out httpapi.Ticket
	code, h := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, &out)
	if code != http.StatusOK || out.Version != 2 || h.Get("ETag") != `"2"` || out.Type != httpapi.TicketTypeFeature {
		t.Fatalf("first save: %d %+v", code, out)
	}
	var p httpapi.Problem
	if code, _ := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, &p); code != http.StatusPreconditionFailed || p.Code != "stale" {
		t.Fatalf("stale save (AC-TK-5): %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodPut, path, edit, nil); code != http.StatusBadRequest {
		t.Fatalf("no If-Match: %d", code)
	}
	delete(edit, "requester_user_id")
	if code, _ := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"2"`}, edit, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "requester_contact_id" {
		t.Fatalf("a replacement names the requester: %d %+v", code, p)
	}
}

func TestUpdatesRecordOldAndNewValues(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	edit := map[string]any{"title": "Overtime export for payroll", "type": "change_request", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{}}
	if code, _ := e.callWith(w.pm, http.MethodPut, "/tickets/"+tk.Key, map[string]string{"If-Match": `"1"`}, edit, nil); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	events, err := e.q.ListTicketEvents(context.Background(), tk.ID)
	if err != nil || len(events) != 1 || events[0].Action != "update" {
		t.Fatalf("history: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["title"]["old"] != "Overtime export" || changes["title"]["new"] != "Overtime export for payroll" ||
		len(changes["menus"]["old"].([]any)) != 1 || len(changes["menus"]["new"].([]any)) != 0 || changes["client"] != nil {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}

func TestTransitionsMoveBetweenOpenStatuses(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	viewer, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var st httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	path := "/tickets/" + tk.Key + "/transition"
	var out httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[1].Id}, &out); code != http.StatusOK || out.Status.Name != "In progress" || out.Version != 2 {
		t.Fatalf("to In progress: %d %+v", code, out)
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": st.Items[3].Id}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_validation_failed" {
		t.Fatalf("to Done without the close fields: %d %+v", code, p)
	}
	if code := e.call(viewer, http.MethodPost, path, map[string]any{"status_id": st.Items[0].Id}, nil); code != http.StatusForbidden {
		t.Fatalf("a viewer moves it: %d", code)
	}
	// AC-TK-1: the history shows "Status To do → In progress" with the member and time.
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if len(events) != 1 || events[0].Action != "transition" || json.Unmarshal(events[0].Changes, &changes) != nil ||
		changes["status"]["old"] != "To do" || changes["status"]["new"] != "In progress" ||
		events[0].ActorName == nil || *events[0].ActorName != "pm@example.com" {
		t.Fatalf("history: %+v", events)
	}
}

func TestHiddenTicketsLookMissing(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Client B payroll change", &w.b)
	outsider, _ := e.signedIn("out@example.com", false)
	for _, c := range []*http.Client{w.pm, outsider} {
		if code := e.call(c, http.MethodGet, "/tickets/"+tk.Key, nil, nil); code != http.StatusNotFound {
			t.Errorf("hidden ticket: %d", code)
		}
	}
}
