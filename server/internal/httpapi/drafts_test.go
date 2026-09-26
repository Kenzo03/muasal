package httpapi_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
)

// §9.3, AC-DC-6, AC-DC-7: "Draft with AI" drafts from the thread only, saves
// nothing, leaves Why empty when the thread never says why, and the close
// keeps ai_drafted.
func TestDraftDecisionFromTheThread(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	var tk httpapi.Ticket
	e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "change_request", "title": "Skip supervisor approval", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID,
		"requester_contact_id": w.budi, "reason": "Asked by Client A in the weekly call.",
	}, &tk)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Budi's email: supervisors are often on leave, so HR approves overtime."}, nil)
	path := "/tickets/" + tk.Key + "/decision-draft"

	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{}, &p); code != http.StatusConflict || p.Code != "ai_off" {
		t.Fatalf("AI off: %d %+v", code, p)
	}

	fake := e.localAI(admin)
	fake.Set(func(s *llmtest.Server) {
		s.Answer = func(system, user string, _ json.RawMessage) string {
			if !strings.Contains(user, "supervisors are often on leave") || !strings.Contains(user, "Overtime Approval") || !strings.Contains(system, "English") {
				return `{"what_changed":"","why":"","alternatives_rejected":""}`
			}
			return `{"what_changed":"HR approves overtime for Client A without a supervisor.","why":"","alternatives_rejected":"Keeping supervisor approval."}`
		}
	})
	var d httpapi.DecisionDraft
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"language": "en"}, &d); code != http.StatusOK ||
		d.WhatChanged != "HR approves overtime for Client A without a supervisor." || d.Why != "" || d.Alternatives != "Keeping supervisor approval." ||
		!strings.HasPrefix(d.Model, "Local") {
		t.Fatalf("draft: %d %+v", code, d)
	}
	var got httpapi.Ticket
	if e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got); got.Decision != nil {
		t.Fatalf("a draft saved something: %+v", got.Decision)
	}

	// The person completes Why and closes; the record keeps ai_drafted.
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]any{
		"status_id": statusID(e, w.pm, "Done"),
		"decision": map[string]any{"what_changed": d.WhatChanged, "why": "Client A supervisors are often on leave.", "alternatives": d.Alternatives, "ai_drafted": true},
	}, &got)
	if got.Decision == nil || got.Decision.AiDrafted == nil || !*got.Decision.AiDrafted || got.Decision.State != httpapi.DecisionStateConfirmed {
		t.Fatalf("closed: %+v", got.Decision)
	}

	fake.Set(func(s *llmtest.Server) { s.Down = true })
	if code := e.call(w.pm, http.MethodPost, path, nil, &p); code != http.StatusServiceUnavailable || p.Code != "ai_unavailable" {
		t.Fatalf("model down: %d %+v", code, p)
	}
}

// §12.1, AC-TK-10: the builder previews closed items by menu; unticked items
// and Internal comments never reach the model; the appendix comes from the
// database; only the creator and project admins read the summary.
func TestChangeSummary(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	pa, paUser := e.signedIn("lead@example.com", false)
	e.seedMember(paUser, w.p, "admin")
	other, otherUser := e.signedIn("rina@example.com", false)
	e.seedMember(otherUser, w.p, "member", w.a)
	done, cancelled := statusID(e, w.pm, "Done"), statusID(e, w.pm, "Cancelled")

	a := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	e.seedClose(a, done, w.pmUser, "HR approves overtime directly.")
	b := e.seedTicket(w.p, w.pmUser, "Overtime cap per month", nil, w.ot)
	e.seedClose(b, done, w.pmUser, "Overtime is capped at 40 hours.")
	c := e.seedTicket(w.p, w.pmUser, "Unticked internal fix", &w.a, w.ot)
	e.seedClose(c, done, w.pmUser, "Refactored the approval query.")
	x := e.seedTicket(w.p, w.pmUser, "Declined multi-level approval", &w.a, w.ot)
	e.seedClose(x, cancelled, w.pmUser, "Multi-level approval was declined.")
	e.seedTicket(w.p, w.pmUser, "Still open", &w.a, w.ot)
	hidden := e.seedTicket(w.p, w.pmUser, "Client B report fix", &w.b, w.secret)
	e.seedClose(hidden, done, w.pmUser, "Client B's report shows totals.")
	ctx := t.Context()
	for _, cm := range []struct {
		body     string
		internal bool
	}{{"INTERNAL: the vendor was slow.", true}, {"Client-safe: rolled out on Monday.", false}} {
		if _, err := e.q.CreateComment(ctx, db.CreateCommentParams{TicketID: a.ID, AuthorID: w.pmUser.ID, Internal: cm.internal, Body: cm.body}); err != nil {
			t.Fatal(err)
		}
	}

	today := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	scope := map[string]any{"project_key": "HRIS", "node_id": w.hr.ID, "client_id": w.a.ID, "from": "2020-01-01", "to": today,
		"language": "en", "audience": "client"}
	var prev httpapi.SummaryPreview
	if code := e.call(w.pm, http.MethodPost, "/summaries/preview", scope, &prev); code != http.StatusOK || len(prev.Items) != 3 || prev.Model != "" {
		t.Fatalf("preview: %d %+v", code, prev)
	}
	for _, it := range prev.Items {
		if it.Menu != "Overtime Approval" || it.Title == "Still open" || it.Title == "Declined multi-level approval" {
			t.Fatalf("item: %+v", it)
		}
	}
	delete(scope, "client_id") // all clients: still only the PM's scope (R-AC-3)
	if e.call(w.pm, http.MethodPost, "/summaries/preview", scope, &prev); len(prev.Items) != 3 {
		t.Fatalf("all clients: %+v", prev.Items)
	}
	scope["client_id"] = w.a.ID
	scope["include_cancelled"] = true
	if e.call(w.pm, http.MethodPost, "/summaries/preview", scope, &prev); len(prev.Items) != 4 {
		t.Fatalf("with cancelled: %+v", prev.Items)
	}

	fake := e.localAI(admin)
	var prompt string
	fake.Set(func(s *llmtest.Server) {
		s.Answer = func(_, user string, _ json.RawMessage) string {
			prompt = user
			return `{"overview":"Overtime approval got simpler for Client A.","bullets":[` +
				`{"text":"HR approves overtime directly.","why":"Supervisors are often on leave.","cites":["` + a.Key + `"]},` +
				`{"text":"An invented change.","why":"","cites":[]}]}`
		}
	})
	create := map[string]any{"keys": []string{a.Key, b.Key, x.Key}}
	for k, v := range scope {
		create[k] = v
	}
	var sum httpapi.Summary
	if code := e.call(w.pm, http.MethodPost, "/summaries", create, &sum); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	switch {
	case strings.Contains(prompt, "INTERNAL") || strings.Contains(prompt, "Unticked"):
		t.Fatalf("the model saw an Internal comment or an unticked item:\n%s", prompt)
	case !strings.Contains(prompt, "Client-safe: rolled out") || !strings.Contains(prompt, "Overtime is capped"):
		t.Fatalf("the model missed ticked evidence:\n%s", prompt)
	}
	md := sum.Markdown
	for _, want := range []string{"## Overtime Approval", "HR changes for Client A", "Overtime approval got simpler", "HR approves overtime directly. Why: Supervisors are often on leave. (" + a.Key + ")",
		"| " + b.Key + " | Overtime cap per month |", "## Appendix: items"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown lacks %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "invented") || strings.Contains(md, "](/t/") || len(sum.Items) != 3 {
		t.Fatalf("markdown: %s %+v", md, sum.Items)
	}

	path := "/summaries/" + strconv.FormatInt(sum.Id, 10)
	if code := e.call(other, http.MethodGet, path, nil, nil); code != http.StatusNotFound {
		t.Fatalf("another member read it: %d", code)
	}
	var edited httpapi.Summary
	if code := e.call(pa, http.MethodPatch, path, map[string]any{"title": "Edited", "markdown": "# Edited"}, &edited); code != http.StatusOK || edited.Markdown != "# Edited" {
		t.Fatalf("project admin edit: %d %+v", code, edited)
	}
	var list httpapi.SummaryList
	if e.call(other, http.MethodGet, "/summaries", nil, &list); len(list.Items) != 0 {
		t.Fatalf("another member's list: %+v", list)
	}
	if e.call(w.pm, http.MethodGet, "/summaries?project=HRIS", nil, &list); len(list.Items) != 1 || list.Items[0].Title != "Edited" {
		t.Fatalf("creator's list: %+v", list)
	}
}
