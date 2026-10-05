package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// statusID finds one of HRIS's statuses by name.
func statusID(e *env, c *http.Client, name string) int64 {
	e.t.Helper()
	var st httpapi.StatusList
	e.call(c, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	for _, s := range st.Items {
		if s.Name == name {
			return s.Id
		}
	}
	e.t.Fatalf("no status %q", name)
	return 0
}

func decisionBody(whatChanged, why string) map[string]any {
	return map[string]any{"what_changed": whatChanged, "why": why, "alternatives": "Backup supervisor, rejected: Client A has no such role."}
}

// ticketActions lists a ticket's history actions, oldest first.
func ticketActions(e *env, ticketID int64) []string {
	events, err := e.q.ListTicketEvents(context.Background(), ticketID)
	if err != nil {
		e.t.Fatal(err)
	}
	out := []string{}
	for _, ev := range events {
		out = append(out, ev.Action)
	}
	return out
}

// AC-DC-1 and R-DC-1 at API level: a close commits the status, the reason, the
// menus and a confirmed decision record together; R-DC-7: without them it is refused.
func TestClosingRecordsTheDecision(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a) // no reason, no menus yet
	path := "/tickets/" + tk.Key + "/transition"
	done := statusID(e, w.pm, "Done")

	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": done}, &p); code != http.StatusUnprocessableEntity || p.Code != "close_validation_failed" {
		t.Fatalf("close without its fields: %d %+v", code, p)
	}
	var fields []string
	for _, f := range *p.Errors {
		fields = append(fields, f.Field+":"+f.Code)
	}
	if want := []string{"reason:required", "node_ids:min_items", "decision.what_changed:required", "decision.why:required"}; !slices.Equal(fields, want) {
		t.Fatalf("field errors: %v, want %v", fields, want)
	}

	var out httpapi.Ticket
	body := map[string]any{
		"status_id": done, "reason": "Client A supervisors are often on leave; HR approves overtime directly.",
		"node_ids": []int64{w.ot.ID},
		"decision": decisionBody("Overtime approval skips the supervisor step for Client A.", "Approvals stalled for days during leave periods."),
	}
	code, h := e.callWith(w.pm, http.MethodPost, path, map[string]string{"If-Match": `"1"`}, body, &out)
	if code != http.StatusOK || out.Status.Name != "Done" || out.ClosedAt == nil || len(out.Nodes) != 1 || h.Get("ETag") != `"2"` ||
		out.Reason != "Client A supervisors are often on leave; HR approves overtime directly." {
		t.Fatalf("close: %d %+v", code, out)
	}
	d := out.Decision
	if d == nil || d.State != httpapi.DecisionStateConfirmed || d.Outcome != httpapi.DecisionOutcomeImplemented ||
		d.ConfirmedBy == nil || d.ConfirmedBy.Id != w.pmUser.ID || d.ConfirmedAt == nil ||
		d.WhatChanged != "Overtime approval skips the supervisor step for Client A." {
		t.Fatalf("decision: %+v", d)
	}
	if got := ticketActions(e, tk.ID); !slices.Equal(got, []string{"transition", "decision_confirm"}) {
		t.Fatalf("history: %v", got)
	}
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if json.Unmarshal(events[0].Changes, &changes) != nil || changes["status"]["new"] != "Done" ||
		changes["reason"]["old"] != "" || changes["menus"] == nil {
		t.Fatalf("the close's changes: %s", events[0].Changes)
	}
}

// R-DC-2 and R-DC-6: Cancelled records a rejection, a reopen turns the record
// back into a draft (AC-DC-3), and the next close confirms it again.
func TestReopeningDraftsTheDecisionAndTheNextCloseConfirmsIt(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Weekend overtime at double rate", &w.a, w.ot)
	path := "/tickets/" + tk.Key + "/transition"
	closeAs := func(status, whatChanged string) httpapi.Ticket {
		t.Helper()
		var out httpapi.Ticket
		body := map[string]any{"status_id": statusID(e, w.pm, status), "reason": "Client A asked for weekend double pay.",
			"decision": decisionBody(whatChanged, "Their budget has no room for it this year.")}
		if code := e.call(w.pm, http.MethodPost, path, body, &out); code != http.StatusOK {
			t.Fatalf("close as %s: %d", status, code)
		}
		return out
	}
	if out := closeAs("Cancelled", "Weekend overtime stays at the normal rate."); out.Decision.Outcome != httpapi.DecisionOutcomeRejected {
		t.Fatalf("cancelled: %+v", out.Decision)
	}
	var out httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"status_id": statusID(e, w.pm, "In progress")}, &out); code != http.StatusOK ||
		out.ClosedAt != nil || out.Decision == nil || out.Decision.State != httpapi.DecisionStateDraft || out.Decision.ConfirmedBy != nil ||
		out.Decision.WhatChanged != "Weekend overtime stays at the normal rate." {
		t.Fatalf("reopen: %d %+v", code, out)
	}
	if out := closeAs("Done", "Weekend overtime pays double from March."); out.Decision.State != httpapi.DecisionStateConfirmed ||
		out.Decision.Outcome != httpapi.DecisionOutcomeImplemented || out.Decision.WhatChanged != "Weekend overtime pays double from March." {
		t.Fatalf("closed again: %+v", out.Decision)
	}
	want := []string{"transition", "decision_confirm", "transition", "decision_draft", "transition", "decision_confirm"}
	if got := ticketActions(e, tk.ID); !slices.Equal(got, want) {
		t.Fatalf("history: %v, want %v", got, want)
	}
}

// AC-TK-5 for closes: a stale If-Match changes nothing.
func TestStaleClosesAreRefused(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime cap of 40 hours", &w.a, w.ot)
	body := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The labor agreement caps overtime.",
		"decision": decisionBody("Overtime is capped at 40 hours a month.", "The labor agreement sets the cap.")}
	var p httpapi.Problem
	if code, _ := e.callWith(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]string{"If-Match": `"7"`}, body, &p); code != http.StatusPreconditionFailed || p.Code != "stale" {
		t.Fatalf("stale close: %d %+v", code, p)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Status.Name != "To do" || got.Decision != nil || got.Reason != "" || len(ticketActions(e, tk.ID)) != 0 {
		t.Fatalf("after a stale close: %+v", got)
	}
}

// AC-DC-5: when the database refuses part of a close, nothing changes: the
// status, the reason, the decision record and the history stay as they were.
func TestAFailedCloseChangesNothing(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, e.d.OwnerURL)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	for _, sql := range []string{
		`CREATE FUNCTION refuse_decision() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'disk full'; END $$`,
		`CREATE TRIGGER refuse_decision BEFORE INSERT ON decision_records FOR EACH ROW EXECUTE FUNCTION refuse_decision()`,
	} {
		if _, err := owner.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	body := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The payroll team imports overtime monthly.",
		"decision": decisionBody("Overtime exports as CSV for payroll.", "Payroll imports it every month.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", body, nil); code != http.StatusInternalServerError {
		t.Fatalf("close: %d", code)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Status.Name != "To do" || got.ClosedAt != nil || got.Decision != nil || got.Reason != "" || got.Version != 1 ||
		len(ticketActions(e, tk.ID)) != 0 {
		t.Fatalf("after a failed close: %+v", got)
	}
}
