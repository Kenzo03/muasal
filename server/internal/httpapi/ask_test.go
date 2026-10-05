package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
	"github.com/kenzo03/zettra/server/internal/indexer"
	"github.com/kenzo03/zettra/server/internal/llm/llmtest"
)

// indexNow indexes tickets the way the workers would.
func (e *env) indexNow(ids ...int64) {
	e.t.Helper()
	ix := indexer.New(e.d.Pool, e.api.AI())
	for _, id := range ids {
		if err := ix.Rebuild(context.Background(), id); err != nil {
			e.t.Fatal(err)
		}
		if err := ix.EmbedTicket(context.Background(), id); err != nil {
			e.t.Fatal(err)
		}
	}
}

// localAI switches the install to Local against a fake model server that
// answers with one claim citing the first evidence key.
func (e *env) localAI(admin *http.Client) *llmtest.Server {
	e.t.Helper()
	fake := llmtest.New(e.t)
	fake.Answer = func(_, _ string, schema json.RawMessage) string {
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
		keys := s.Properties.Claims.Items.Properties.Cites.Items.Enum
		return fmt.Sprintf(`{"claims":[{"text":"HR approves overtime for Client A.","cites":[%q]}]}`, keys[0])
	}
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", aiUpdate("local", fake.BaseURL()), nil); code != http.StatusOK {
		e.t.Fatalf("switch to Local: %d", code)
	}
	return fake
}

type sseEvent struct {
	name string
	data json.RawMessage
}

// askStream posts a question with Accept: text/event-stream and returns its events.
func (e *env) askStream(c *http.Client, body map[string]any) []sseEvent {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/ask", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		e.t.Fatalf("stream: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var out []sseEvent
	var cur sseEvent
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			cur.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			cur.data = json.RawMessage(strings.TrimPrefix(line, "data: "))
		case line == "" && cur.name != "":
			out = append(out, cur)
			cur = sseEvent{}
		}
	}
	return out
}

// AC-AK-2 and §11.6: the stream sends the scope, the evidence, each claim and
// the result, and every chip names a ticket the asker can open.
func TestAskStreamsTheAnswer(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	e.localAI(admin)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime approval skips the supervisor", &w.a, w.ot)
	e.indexNow(tk.ID)
	events := e.askStream(w.pm, map[string]any{"question": "Why does overtime approval skip the supervisor for Client A?",
		"scope": map[string]any{"node_ids": []int64{w.hr.ID}}})
	var names []string
	for _, ev := range events {
		names = append(names, ev.name)
	}
	if strings.Join(names, ",") != "scope,evidence,claim,result" {
		t.Fatalf("events: %v", names)
	}
	var claim httpapi.AskClaim
	var result struct {
		Status   string `json:"status"`
		QueryID  int64  `json:"query_id"`
		ThreadID int64  `json:"thread_id"`
		Model    string `json:"model"`
	}
	_ = json.Unmarshal(events[2].data, &claim)
	_ = json.Unmarshal(events[3].data, &result)
	if len(claim.Cites) != 1 || claim.Cites[0] != tk.Key || result.Status != "answered" || result.QueryID == 0 || result.Model != "Local · qwen3.5:4b" {
		t.Fatalf("claim %+v, result %+v", claim, result)
	}
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/tickets/%s", claim.Cites[0]), nil, nil); code != http.StatusOK {
		t.Fatalf("the cited ticket does not open: %d", code)
	}
}

// §10.4 and AC-IX-5 in JSON mode: not enough information has the fixed text;
// with AI off the answer is keyword results.
func TestAskAnswersAsJSON(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime approval skips the supervisor", &w.a, w.ot)
	e.indexNow(tk.ID)
	var res httpapi.AskResult
	if code := e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "overtime supervisor"}, &res); code != http.StatusOK ||
		res.Status != httpapi.AskResultStatusAiOff || len(res.Results) != 1 || res.Results[0].Key != tk.Key || res.Model != nil {
		t.Fatalf("AI off: %d %+v", code, res)
	}
	fake := e.localAI(admin)
	fake.Set(func(s *llmtest.Server) {
		s.Answer = func(string, string, json.RawMessage) string { return `{"claims":[]}` }
	})
	if code := e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "Why does overtime skip the supervisor?"}, &res); code != http.StatusOK ||
		res.Status != httpapi.AskResultStatusNotEnoughInfo || res.Message == nil ||
		*res.Message != "Not enough information in the tickets you can access to answer this." || len(res.Closest) != 1 {
		t.Fatalf("not enough information: %d %+v", code, res)
	}
}

// §10.6: threads are private to their owner, and hiding one keeps the log.
func TestThreadsArePrivate(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	other, _ := e.signedIn("ani@example.com", false)
	var res httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "What changed in overtime?"}, &res)
	var second httpapi.AskResult
	e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "And for payroll?", "thread_id": res.ThreadId}, &second)
	var detail httpapi.AskThreadDetail
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/ask/threads/%d", res.ThreadId), nil, &detail); code != http.StatusOK ||
		second.ThreadId != res.ThreadId || len(detail.Queries) != 2 || detail.Title != "What changed in overtime?" {
		t.Fatalf("thread: %d %+v", code, detail)
	}
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		if code := e.call(other, m, fmt.Sprintf("/ask/threads/%d", res.ThreadId), nil, nil); code != http.StatusNotFound {
			t.Fatalf("%s another user's thread: %d", m, code)
		}
	}
	if code := e.call(other, http.MethodPost, "/ask", map[string]any{"question": "x", "thread_id": res.ThreadId}, nil); code != http.StatusNotFound {
		t.Fatalf("asking in another user's thread: %d", code)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/ask/threads/%d", res.ThreadId), nil, nil); code != http.StatusNoContent {
		t.Fatalf("hide: %d", code)
	}
	var list httpapi.AskThreadList
	e.call(w.pm, http.MethodGet, "/ask/threads", nil, &list)
	var logged int
	_ = e.d.Pool.QueryRow(context.Background(), "SELECT count(*) FROM ask_queries WHERE thread_id = $1", res.ThreadId).Scan(&logged)
	if len(list.Items) != 0 || logged != 2 {
		t.Fatalf("after hiding: %d threads listed, %d logged", len(list.Items), logged)
	}
}

// §17.1: 10 questions a minute per user.
func TestAskIsRateLimited(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	for i := 0; i < 10; i++ {
		if code := e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "What changed?"}, nil); code != http.StatusOK {
			t.Fatalf("question %d: %d", i+1, code)
		}
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/ask", map[string]any{"question": "What changed?"}, &p); code != http.StatusTooManyRequests || p.Code != "rate_limited" {
		t.Fatalf("the 11th question: %d %+v", code, p)
	}
}
