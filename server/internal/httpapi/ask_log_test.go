package httpapi_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// §10.3 and §10.6: a thread shows each answer's evidence again, so its
// citation chips keep their title, client and dates.
func TestThreadShowsTheEvidence(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	e.localAI(admin)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime approval skips the supervisor", &w.a, w.ot)
	e.indexNow(tk.ID)
	var res httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Why does overtime approval skip the supervisor?"}, &res)
	var detail httpapi.AskThreadDetail
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/ask/threads/%d", res.ThreadId), nil, &detail); code != http.StatusOK {
		t.Fatalf("thread: %d", code)
	}
	q := detail.Queries[0]
	if q.Evidence == nil || len(*q.Evidence) != 1 || (*q.Evidence)[0].Key != tk.Key || (*q.Evidence)[0].Title != tk.Title {
		t.Fatalf("evidence: %+v", q.Evidence)
	}
}

// §15.4 and §10.8: system admins read every question with its scope,
// evidence and answer; quick filters find the unanswered and the slow ones.
func TestAskLogIsForSystemAdmins(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime approval skips the supervisor", &w.a, w.ot)
	e.indexNow(tk.ID)
	var off httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "overtime supervisor"}, &off) // AI is off
	e.localAI(admin)
	var answered, none httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Why does overtime approval skip the supervisor?"}, &answered)
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Kapan kantin buka?"}, &none)
	if answered.Status != "answered" || none.Status != "not_enough_info" {
		t.Fatalf("statuses: %s %s", answered.Status, none.Status)
	}
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE ask_queries SET latency_ms = 31000 WHERE id = $1", answered.QueryId); err != nil {
		t.Fatal(err)
	}

	var page httpapi.AskLogPage
	if code := e.call(admin, http.MethodGet, "/admin/ask-log", nil, &page); code != http.StatusOK || len(page.Items) != 3 {
		t.Fatalf("log: %d %+v", code, page)
	}
	if page.Items[0].Id != none.QueryId || page.Items[0].User.Name != "pm@example.com" || page.Items[0].Question != "Kapan kantin buka?" {
		t.Fatalf("newest first, with the asker: %+v", page.Items[0])
	}
	for filter, want := range map[string]int64{"?status=not_enough_info": none.QueryId, "?slow=true": answered.QueryId, "?status=ai_off": off.QueryId} {
		if code := e.call(admin, http.MethodGet, "/admin/ask-log"+filter, nil, &page); code != http.StatusOK || len(page.Items) != 1 || page.Items[0].Id != want {
			t.Fatalf("%s: %d %+v", filter, code, page.Items)
		}
	}

	var entry httpapi.AskLogDetail
	if code := e.call(admin, http.MethodGet, fmt.Sprintf("/admin/ask-log/%d", answered.QueryId), nil, &entry); code != http.StatusOK {
		t.Fatalf("entry: %d", code)
	}
	if !entry.LlmCalled || len(entry.Evidence) != 1 || entry.Evidence[0].Key != tk.Key || entry.Evidence[0].Score == nil ||
		len(entry.Claims) == 0 || entry.Model == nil || *entry.Model != "Local · qwen3.5:4b" || entry.Citations != 1 {
		t.Fatalf("entry: %+v", entry)
	}
	if code := e.call(admin, http.MethodGet, fmt.Sprintf("/admin/ask-log/%d", none.QueryId), nil, &entry); code != http.StatusOK || entry.LlmCalled {
		t.Fatalf("AC-AK-5: the log shows the chat model was not called: %+v", entry)
	}

	for _, path := range []string{"/admin/ask-log", fmt.Sprintf("/admin/ask-log/%d", answered.QueryId)} {
		if code := e.call(w.pm, http.MethodGet, path, nil, nil); code != http.StatusForbidden {
			t.Fatalf("%s as a member: %d", path, code)
		}
	}
	if code := e.call(admin, http.MethodGet, "/admin/ask-log/999999", nil, nil); code != http.StatusNotFound {
		t.Fatalf("missing entry: %d", code)
	}
}

// §15.4: the audit log filters by actor, entity, action and date, read-only,
// with a CSV export.
func TestAuditLogFiltersAndExports(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, adminUser := e.signedIn("admin@example.com", true)
	if code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": "Cahaya Farma"}, nil); code != http.StatusCreated {
		t.Fatalf("client: %d", code)
	}
	var page httpapi.AuditPage
	if code := e.call(admin, http.MethodGet, "/admin/audit?entity=client&action=create", nil, &page); code != http.StatusOK || len(page.Items) != 1 {
		t.Fatalf("filtered: %d %+v", code, page)
	}
	ev := page.Items[0]
	if ev.Actor == nil || ev.Actor.Name != adminUser.Name || ev.Entity != "client" || ev.Action != "create" || ev.Via != "web" {
		t.Fatalf("event: %+v", ev)
	}
	if code := e.call(admin, http.MethodGet, fmt.Sprintf("/admin/audit?actor_id=%d&entity=client", w.pmUser.ID), nil, &page); code != http.StatusOK || len(page.Items) != 0 {
		t.Fatalf("by another actor: %+v", page.Items)
	}
	if code := e.call(admin, http.MethodGet, "/admin/audit?from=2000-01-01&to=2000-01-02", nil, &page); code != http.StatusOK || len(page.Items) != 0 {
		t.Fatalf("by date: %+v", page.Items)
	}

	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/admin/audit/export?entity=client", nil)
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") || err != nil ||
		len(rows) != 2 || strings.Join(rows[0], ",") != "id,occurred_at,actor,via,entity,entity_id,project,action,changes,token,subject" || rows[1][4] != "client" ||
		rows[1][10] != "Cahaya Farma" {
		t.Fatalf("export: %d %q %v %q", resp.StatusCode, resp.Header.Get("Content-Type"), err, body)
	}
	for _, path := range []string{"/admin/audit", "/admin/audit/export"} {
		if code := e.call(w.pm, http.MethodGet, path, nil, nil); code != http.StatusForbidden {
			t.Fatalf("%s as a member: %d", path, code)
		}
	}
}

// Names and other free text in the audit export stay text in a spreadsheet.
func TestAuditExportKeepsFormulasAsText(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	e.exec("UPDATE users SET name = '=HYPERLINK(1)' WHERE email = 'admin@example.com'")
	if code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": "=SUM(1)"}, nil); code != http.StatusCreated {
		t.Fatalf("client: %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/admin/audit/export?entity=client", nil)
	resp, err := admin.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	rows, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil || len(rows) != 2 || rows[1][2] != "'=HYPERLINK(1)" || rows[1][10] != "'=SUM(1)" {
		t.Fatalf("export: %v %q", err, body)
	}
}

// §10.2: one click removes a detected chip and re-runs the question; the
// request names the chips to leave out, so detection does not bring them back.
func TestRemovedDetectedChipsStayOff(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	q := "Why does overtime approval skip the supervisor for Client A in 2026?"
	var first, again httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": q}, &first)
	d := first.Scope.Detected
	if d.ClientIds == nil || len(*d.ClientIds) != 1 || d.NodeIds == nil || d.From == nil || d.Labels == nil || len(*d.Labels) != 2 {
		t.Fatalf("detected: %+v", d)
	}
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": q, "ignore": []map[string]any{
		{"kind": "client", "id": w.a.ID}, {"kind": "date"},
	}}, &again)
	d = again.Scope.Detected
	if d.ClientIds != nil || d.From != nil || d.To != nil || d.NodeIds == nil || len(*d.Labels) != 1 || (*d.Labels)[0].Kind != "node" {
		t.Fatalf("after removing the client and the dates: %+v", d)
	}
}

// AC-AK-8: a thumbs-down with reason "Wrong citation" shows in the Ask log and
// its thumbs-down filter; only the asker rates, and a later rating replaces it.
func TestThumbsDownReachesTheAskLog(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	e.localAI(admin)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime approval skips the supervisor", &w.a, w.ot)
	e.indexNow(tk.ID)
	var up, down httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Why does overtime approval skip the supervisor?"}, &up)
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Who asked to skip the supervisor for overtime?"}, &down)

	path := fmt.Sprintf("/ask/queries/%d/feedback", down.QueryId)
	if code := e.call(admin, http.MethodPost, path, map[string]any{"rating": "up"}, nil); code != http.StatusNotFound {
		t.Fatalf("someone else's question: %d", code)
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"rating": "meh"}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad rating: %d", code)
	}
	e.call(w.pm, http.MethodPost, path, map[string]any{"rating": "up"}, nil)
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"rating": "down", "reasons": []string{"wrong_citation"}, "comment": "Cites the wrong ticket."}, nil); code != http.StatusNoContent {
		t.Fatalf("down: %d", code)
	}
	e.call(w.pm, http.MethodPost, fmt.Sprintf("/ask/queries/%d/feedback", up.QueryId), map[string]any{"rating": "up", "reasons": []string{"wrong"}}, nil)

	var page httpapi.AskLogPage
	e.call(admin, http.MethodGet, "/admin/ask-log?down=true", nil, &page)
	if len(page.Items) != 1 || page.Items[0].Id != down.QueryId {
		t.Fatalf("thumbs-down filter: %+v", page.Items)
	}
	f := page.Items[0].Feedback
	if f == nil || f.Rating != httpapi.AskFeedbackRatingDown || f.Reasons == nil || len(*f.Reasons) != 1 || (*f.Reasons)[0] != httpapi.AskFeedbackReasonsWrongCitation ||
		f.Comment == nil || *f.Comment != "Cites the wrong ticket." {
		t.Fatalf("feedback: %+v", f)
	}
	var detail httpapi.AskThreadDetail
	e.call(w.pm, http.MethodGet, fmt.Sprintf("/ask/threads/%d", up.ThreadId), nil, &detail)
	if fb := detail.Queries[0].Feedback; fb == nil || fb.Rating != httpapi.AskFeedbackRatingUp || len(*fb.Reasons) != 0 {
		t.Fatalf("thread feedback: %+v", fb)
	}
}
