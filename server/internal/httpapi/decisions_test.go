package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// R-DC-5: project admins and the confirmer reword a confirmed record, and every
// edit keeps the earlier words in the ticket's history.
func TestOnlyAdminsAndTheConfirmerEditADecision(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	other, ou := e.signedIn("ani@example.com", false)
	e.seedMember(ou, w.p, "member")
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	path := "/tickets/" + tk.Key + "/decision"
	edit := map[string]any{"what_changed": "Overtime approval skips the supervisor for Client A.", "why": "Supervisors are often on leave."}

	var early httpapi.Problem
	if code := e.call(w.pm, http.MethodPut, path, edit, &early); code != http.StatusConflict || early.Code != "decision_not_confirmed" {
		t.Fatalf("an edit before the close: %d %+v", code, early)
	}
	closeBody := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "Client A supervisors are often on leave.",
		"decision": decisionBody("Overtime approval skips the supervisor.", "Supervisors are often on leave.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", closeBody, nil); code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	if code := e.call(other, http.MethodPut, path, edit, nil); code != http.StatusForbidden {
		t.Fatalf("another member edits: %d", code)
	}
	for _, c := range []*http.Client{w.pm, admin} { // the confirmer, then a project admin
		var out httpapi.DecisionRecord
		if code := e.call(c, http.MethodPut, path, edit, &out); code != http.StatusOK || out.WhatChanged != edit["what_changed"] ||
			out.Alternatives != "" || out.ConfirmedBy == nil || out.ConfirmedBy.Id != w.pmUser.ID {
			t.Fatalf("edit: %d %+v", code, out)
		}
	}
	var short httpapi.Problem
	if code := e.call(admin, http.MethodPut, path, map[string]any{"what_changed": "Short", "why": "Supervisors are often on leave."}, &short); code != http.StatusUnprocessableEntity || firstError(short).Field != "what_changed" {
		t.Fatalf("a short edit: %d %+v", code, short)
	}
	// The admin's edit repeated the pm's words, so the history holds one edit.
	if got := ticketActions(e, tk.ID); !slices.Equal(got, []string{"transition", "decision_confirm", "decision_edit"}) {
		t.Fatalf("history: %v", got)
	}
	events, _ := e.q.ListTicketEvents(context.Background(), tk.ID)
	var changes map[string]map[string]any
	if json.Unmarshal(events[2].Changes, &changes) != nil || changes["what_changed"]["old"] != "Overtime approval skips the supervisor." ||
		changes["alternatives"]["old"] != "Backup supervisor, rejected: Client A has no such role." {
		t.Fatalf("the edit's changes: %s", events[2].Changes)
	}
}

// TK-3 holds after the close: a closed ticket keeps a reason and a menu.
func TestClosedTicketsKeepTheirReasonAndMenus(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime cap of 40 hours", &w.a, w.ot)
	closeBody := map[string]any{"status_id": statusID(e, w.pm, "Done"), "reason": "The labor agreement caps overtime.",
		"decision": decisionBody("Overtime is capped at 40 hours a month.", "The labor agreement sets the cap.")}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", closeBody, nil); code != http.StatusOK {
		t.Fatalf("close: %d", code)
	}
	edit := map[string]any{"type": "change_request", "title": "Overtime cap of 40 hours", "client_id": w.a.ID,
		"requester_user_id": w.pmUser.ID, "node_ids": []int64{}, "reason": ""}
	var p httpapi.Problem
	code, _ := e.callWith(w.pm, http.MethodPut, "/tickets/"+tk.Key, map[string]string{"If-Match": `"2"`}, edit, &p)
	var fields []string
	if p.Errors != nil {
		for _, f := range *p.Errors {
			fields = append(fields, f.Field+":"+f.Code)
		}
	}
	if code != http.StatusUnprocessableEntity || !slices.Equal(fields, []string{"reason:required", "node_ids:min_items"}) {
		t.Fatalf("emptying a closed ticket: %d %v", code, fields)
	}
}
