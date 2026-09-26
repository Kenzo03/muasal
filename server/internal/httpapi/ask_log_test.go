package httpapi_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
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
		len(rows) != 2 || strings.Join(rows[0], ",") != "id,occurred_at,actor,via,entity,entity_id,project,action,changes" || rows[1][4] != "client" {
		t.Fatalf("export: %d %q %v %q", resp.StatusCode, resp.Header.Get("Content-Type"), err, body)
	}
	for _, path := range []string{"/admin/audit", "/admin/audit/export"} {
		if code := e.call(w.pm, http.MethodGet, path, nil, nil); code != http.StatusForbidden {
			t.Fatalf("%s as a member: %d", path, code)
		}
	}
}
