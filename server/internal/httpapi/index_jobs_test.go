package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// indexJobs counts the index_ticket jobs queued for a ticket.
func indexJobs(e *env, ticketID int64) int {
	e.t.Helper()
	var n int
	if err := e.d.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM river_job WHERE kind = 'index_ticket' AND (args->>'ticket_id')::bigint = $1", ticketID).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// FSD §13.2: every change a ticket's chunks show queues one index job in the
// change's transaction; a refused change queues none.
func TestEveryChangeQueuesAnIndexJob(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	var tk httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "change_request", "title": "Skip supervisor approval", "client_id": w.a.ID, "requester_contact_id": w.budi,
		"node_ids": []int64{w.ot.ID}, "reason": "Supervisors at Client A are often on leave.",
	}, &tk); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	path := "/tickets/" + tk.Key
	want := 1
	step := func(name string, code, wantCode int) {
		t.Helper()
		if code != wantCode {
			t.Fatalf("%s: %d, want %d", name, code, wantCode)
		}
		if wantCode/100 == 2 {
			want++
		}
		if got := indexJobs(e, tk.Id); got != want {
			t.Fatalf("after %s: %d index jobs, want %d", name, got, want)
		}
	}
	if got := indexJobs(e, tk.Id); got != 1 {
		t.Fatalf("after create: %d index jobs", got)
	}

	edit := map[string]any{"type": "change_request", "title": "Skip supervisor approval for overtime", "client_id": w.a.ID,
		"requester_contact_id": w.budi, "node_ids": []int64{w.ot.ID}, "reason": "Supervisors at Client A are often on leave."}
	code, _ := e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, nil)
	step("update", code, 200)
	code, _ = e.callWith(w.pm, http.MethodPut, path, map[string]string{"If-Match": `"1"`}, edit, nil)
	step("a stale update", code, 412)

	var c httpapi.ActivityItem
	step("comment", e.call(w.pm, http.MethodPost, path+"/comments", map[string]any{"body": "Confirmed by phone."}, &c), 201)
	step("comment edit", e.call(w.pm, http.MethodPatch, fmt.Sprintf("/comments/%d", *c.CommentId), map[string]any{"body": "Confirmed with Budi by phone."}, nil), 200)
	step("comment delete", e.call(w.pm, http.MethodDelete, fmt.Sprintf("/comments/%d", *c.CommentId), nil, nil), 204)

	closeBody := map[string]any{"status_id": statusID(e, w.pm, "Done"),
		"decision": decisionBody("Overtime approval skips the supervisor.", "Supervisors are often on leave.")}
	step("close", e.call(w.pm, http.MethodPost, path+"/transition", closeBody, nil), 200)
	step("decision edit", e.call(w.pm, http.MethodPut, path+"/decision",
		map[string]any{"what_changed": "Overtime approval skips the supervisor for Client A.", "why": "Supervisors are often on leave."}, nil), 200)

	step("menu rename", e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", w.hr.ID), map[string]any{"name": "Human Resources"}, nil), 200)
	step("client rename", e.call(admin, http.MethodPatch, fmt.Sprintf("/clients/%d", w.a.ID), map[string]any{"name": "Client A Group"}, nil), 200)
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", w.hr.ID), map[string]any{"description": "People matters"}, nil); code != 200 || indexJobs(e, tk.Id) != want {
		t.Fatalf("a description edit re-indexes: %d, %d jobs", code, indexJobs(e, tk.Id))
	}
}
