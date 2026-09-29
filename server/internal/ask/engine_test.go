package ask_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
)

// enumOf reads the citation enum from a request's schema.
func enumOf(schema json.RawMessage) []string {
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

// citeFirst answers with one claim citing the first evidence key.
func citeFirst(text string) func(string, string, json.RawMessage) string {
	return func(_, _ string, schema json.RawMessage) string {
		keys := enumOf(schema)
		return fmt.Sprintf(`{"claims":[{"text":%q,"cites":[%q]}]}`, text, keys[0])
	}
}

type recorder struct {
	mu       sync.Mutex
	events   []string
	evidence []ask.Item
}

func (r *recorder) sink() ask.Sink {
	add := func(e string) { r.mu.Lock(); r.events = append(r.events, e); r.mu.Unlock() }
	return ask.Sink{
		Queued:   func(n int) { add(fmt.Sprintf("queued:%d", n)) },
		Scope:    func(ask.Scope, ask.Detected) { add("scope") },
		Evidence: func(items []ask.Item) { r.mu.Lock(); r.evidence = items; r.mu.Unlock(); add("evidence") },
		Claim:    func(ask.Claim) { add("claim") },
	}
}

func (w *world) ask(u db.User, question string, explicit ask.Scope, sink ask.Sink) ask.Result {
	w.t.Helper()
	res, err := ask.NewEngine(w.d.Pool, w.rt).Ask(context.Background(), ask.Request{Asker: w.asker(u), Question: question, Explicit: explicit}, sink)
	w.check(err)
	return res
}

func (w *world) logged(id int64) (llmCalled bool, status, model string) {
	w.t.Helper()
	var m *string
	w.check(w.d.Pool.QueryRow(context.Background(), "SELECT llm_called, status, model FROM ask_queries WHERE id = $1", id).Scan(&llmCalled, &status, &m))
	if m != nil {
		model = *m
	}
	return
}

// AC-AK-1 and AC-AK-2: an Indonesian question on the node page is answered in
// Indonesian from cited evidence, with the node and the detected client as scope.
func TestAnAnsweredQuestion(t *testing.T) {
	w := newWorld(t)
	tk := w.ticket("Approval lembur skip supervisor", &w.a, w.ot, "Supervisor Client A sering cuti.", "2025-06-10", "Approval lembur langsung ke HR untuk Client A.")
	w.fake.Answer = citeFirst("Approval lembur untuk Client A langsung ke HR karena supervisor sering cuti.")
	var rec recorder
	var detected ask.Detected
	sink := rec.sink()
	sink.Scope = func(_ ask.Scope, d ask.Detected) { detected = d; rec.events = append(rec.events, "scope") }
	res := w.ask(w.member, "Kenapa approval lembur skip supervisor untuk Client A?", ask.Scope{NodeIDs: []int64{w.ot.ID}}, sink)
	if res.Status != ask.StatusAnswered || res.Language != "id" || len(res.Claims) != 1 || res.Claims[0].Cites[0] != tk.Key || res.Model != "Local · qwen3.5:4b" {
		t.Fatalf("result: %+v", res)
	}
	if strings.Join(rec.events, ",") != "scope,evidence,claim" || len(detected.ClientIDs) != 1 || detected.ClientIDs[0] != w.a.ID ||
		rec.evidence[0].Key != tk.Key || rec.evidence[0].RequestedBy != "Hana" {
		t.Fatalf("events: %v, detected %+v, evidence %+v", rec.events, detected, rec.evidence)
	}
	if system, _ := w.fake.LastChat["messages"].([]any)[0].(map[string]any)["content"].(string); !strings.Contains(system, "Answer in Bahasa Indonesia") {
		t.Fatalf("system prompt: %q", system)
	}
	if called, status, model := w.logged(res.QueryID); !called || status != "answered" || model != "Local · qwen3.5:4b" {
		t.Fatalf("log: %v %s %s", called, status, model)
	}
}

// AC-AK-5: with nothing relevant, the reply is not enough information and the
// chat model is not called.
func TestNothingRelevantSkipsTheModel(t *testing.T) {
	w := newWorld(t)
	w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	s := must(w.rt.Store.Get(context.Background()))
	s.MinSimilarity = 0.99
	w.check(w.rt.Store.Put(context.Background(), w.q, s, w.admin.ID))
	res := w.ask(w.member, "Quantum zebra marmalade?", ask.Scope{}, ask.Sink{})
	if chat, _ := w.fake.Calls(); res.Status != ask.StatusNotEnough || chat != 0 {
		t.Fatalf("result %+v, chat calls %d", res, chat)
	}
	if called, status, _ := w.logged(res.QueryID); called || status != "not_enough_info" {
		t.Fatalf("log: %v %s", called, status)
	}
}

// AC-AK-6: a claim citing a key outside the evidence is dropped; with none
// left, the reply is not enough information, with the closest tickets.
func TestInventedCitationsAreDropped(t *testing.T) {
	w := newWorld(t)
	tk := w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	w.fake.Answer = func(string, string, json.RawMessage) string {
		return `{"claims":[{"text":"HR approves overtime.","cites":["HRIS-999"]}]}`
	}
	res := w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, ask.Sink{})
	if res.Status != ask.StatusNotEnough || len(res.Claims) != 0 || len(res.Closest) != 1 || res.Closest[0].Key != tk.Key {
		t.Fatalf("result: %+v", res)
	}
	var dropped string
	w.check(w.d.Pool.QueryRow(context.Background(), "SELECT dropped::text FROM ask_queries WHERE id = $1", res.QueryID).Scan(&dropped))
	if called, _, _ := w.logged(res.QueryID); !called || !strings.Contains(dropped, "no_citation") {
		t.Fatalf("log: %v %s", called, dropped)
	}
}

// MSL-4: a why answer that gives a reason drops its own "the reason is not
// recorded" claim, whichever comes first; without a reason, that claim stays.
func TestAWhyAnswerNeverContradictsItsReason(t *testing.T) {
	w := newWorld(t)
	tk := w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	answer := func(texts ...string) func(string, string, json.RawMessage) string {
		return func(string, string, json.RawMessage) string {
			claims := make([]string, len(texts))
			for i, s := range texts {
				claims[i] = fmt.Sprintf(`{"text":%q,"cites":[%q]}`, s, tk.Key)
			}
			return `{"claims":[` + strings.Join(claims, ",") + `]}`
		}
	}
	w.fake.Answer = answer("The evidence does not say why HR approves overtime.", "HR approves overtime because supervisors are on leave.")
	res := w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, ask.Sink{})
	if len(res.Claims) != 1 || !strings.Contains(res.Claims[0].Text, "because") {
		t.Fatalf("with a reason: %+v", res.Claims)
	}
	var dropped string
	w.check(w.d.Pool.QueryRow(context.Background(), "SELECT dropped::text FROM ask_queries WHERE id = $1", res.QueryID).Scan(&dropped))
	if !strings.Contains(dropped, "contradicts_reason") {
		t.Fatalf("log: %s", dropped)
	}
	w.fake.Answer = answer("The evidence does not say why HR approves overtime.", "HR approves overtime.")
	res = w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, ask.Sink{})
	if len(res.Claims) != 2 || res.Claims[1].Text != "The evidence does not say why HR approves overtime." {
		t.Fatalf("without a reason: %+v", res.Claims)
	}
}

// AC-AK-7: an English question about Indonesian tickets is answered in English.
func TestTheQuestionsLanguageWins(t *testing.T) {
	w := newWorld(t)
	w.ticket("Approval lembur skip supervisor", &w.a, w.ot, "Supervisor sering cuti.", "2025-06-10", "Approval lembur langsung ke HR.")
	w.fake.Answer = citeFirst("Overtime approval goes straight to HR.")
	res := w.ask(w.member, "Why does overtime approval skip the supervisor?", ask.Scope{}, ask.Sink{})
	system, _ := w.fake.LastChat["messages"].([]any)[0].(map[string]any)["content"].(string)
	if res.Language != "en" || !strings.Contains(system, "Answer in English") {
		t.Fatalf("language %s, prompt %q", res.Language, system)
	}
}

// AC-AK-10: BYOK answers carry the Cloud badge, in the result and the log.
func TestBYOKAnswersShowTheCloudBadge(t *testing.T) {
	w := newWorld(t)
	w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	s := must(w.rt.Store.Get(context.Background()))
	s.Mode, s.Provider, s.Acknowledged = ai.ModeBYOK, "Example Cloud", true
	w.check(w.rt.Store.Put(context.Background(), w.q, s, w.admin.ID))
	w.fake.Answer = citeFirst("HR approves overtime.")
	res := w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, ask.Sink{})
	if _, _, model := w.logged(res.QueryID); res.Model != "Cloud · Example Cloud · qwen3.5:4b" || model != res.Model {
		t.Fatalf("badge %q, log %q", res.Model, model)
	}
}

// AC-IX-5: with AI off, keyword results under the same scope, and no model call.
func TestOffModeReturnsKeywordResults(t *testing.T) {
	w := newWorld(t)
	tk := w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	s := must(w.rt.Store.Get(context.Background()))
	s.Mode = ai.ModeOff
	w.check(w.rt.Store.Put(context.Background(), w.q, s, w.admin.ID))
	chatBefore, embedBefore := w.fake.Calls()
	res := w.ask(w.member, "overtime supervisor", ask.Scope{}, ask.Sink{})
	chat, embed := w.fake.Calls()
	if res.Status != ask.StatusAIOff || len(res.Results) != 1 || res.Results[0].Key != tk.Key || chat != chatBefore || embed != embedBefore {
		t.Fatalf("result %+v, calls %d/%d", res, chat-chatBefore, embed-embedBefore)
	}
}

// §11.5 and §11.7: invalid JSON is an error the log keeps; a busy model server
// queues the question and says how many are ahead.
func TestInvalidAnswersAndTheQueue(t *testing.T) {
	w := newWorld(t)
	w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "2025-06-10", "HR approves overtime.")
	w.fake.Answer = func(string, string, json.RawMessage) string { return `{"claims":[{"text":"HR` }
	if res := w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, ask.Sink{}); res.Status != ask.StatusError || res.ErrorCode != "ai_invalid" {
		t.Fatalf("invalid JSON: %+v", res)
	}

	w.fake.Answer = citeFirst("HR approves overtime.")
	release, err := w.rt.Gate.Acquire(context.Background(), 1, nil)
	w.check(err)
	var rec recorder
	done := make(chan ask.Result)
	go func() { done <- w.ask(w.member, "Why does HR approve overtime?", ask.Scope{}, rec.sink()) }()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		rec.mu.Lock()
		queued := strings.Contains(strings.Join(rec.events, ","), "queued:1")
		rec.mu.Unlock()
		if queued {
			break
		}
	}
	release()
	res := <-done
	if res.Status != ask.StatusAnswered || !strings.Contains(strings.Join(rec.events, ","), "evidence,queued:1,claim") {
		t.Fatalf("queued: %+v %v", res, rec.events)
	}
}

// AC-DC-8: a decision note on Overtime Approval is evidence Ask can cite, as
// HRIS-DN1; a note for Client B never reaches a member scoped to Client A.
func TestAskCitesDecisionNotes(t *testing.T) {
	w := newWorld(t)
	note := w.note("Overtime approval skips the supervisor", &w.a, w.ot, "2025-11-04",
		"Decision: HR approves overtime for Client A directly.\nWhy: supervisors are often on leave.")
	w.note("Overtime approval needs two supervisors", &w.b, w.ot, "2025-11-05", "Decision: two supervisors approve Client B overtime.")
	w.fake.Answer = citeFirst("HR approves overtime directly because supervisors are often on leave.")
	var rec recorder
	res := w.ask(w.member, "Why does overtime approval skip the supervisor?", ask.Scope{}, rec.sink())
	if res.Status != ask.StatusAnswered || len(res.Claims) != 1 || res.Claims[0].Cites[0] != note.Key {
		t.Fatalf("answer: %+v", res)
	}
	for _, it := range rec.evidence {
		if it.Key == "HRIS-DN2" {
			t.Fatalf("Client B's note reached the member: %+v", rec.evidence)
		}
	}
	if it := rec.evidence[0]; it.Kind != "note" || it.Title != note.Title || it.RequestedBy != "Hana" || it.Date.Format(time.DateOnly) != "2025-11-04" {
		t.Fatalf("evidence item: %+v", it)
	}
}

// AC-AK-9 and §11.9: "And for Client B?" after a Client A answer swaps the
// client chip, keeps the menu, cites only Client B or core tickets, and sends
// the earlier turn as CONVERSATION, all without an extra model call.
func TestFollowUpsSwapTheClient(t *testing.T) {
	w := newWorld(t)
	forA := w.ticket("Overtime approval skips the supervisor", &w.a, w.ot, "Client A supervisors are on leave.", "2025-06-10", "Skip the supervisor for Client A.")
	forB := w.ticket("Overtime approval needs two supervisors", &w.b, w.ot, "Client B wants two approvers.", "2025-07-01", "Two supervisors approve Client B overtime.")
	var user string
	w.fake.Answer = func(_, u string, schema json.RawMessage) string {
		user = u
		return citeFirst("See the evidence.")("", u, schema)
	}
	engine := ask.NewEngine(w.d.Pool, w.rt)
	first, err := engine.Ask(context.Background(), ask.Request{Asker: w.asker(w.admin), Question: "Why does overtime approval skip the supervisor for Client A?"}, ask.Sink{})
	w.check(err)
	if first.Status != ask.StatusAnswered || first.Claims[0].Cites[0] != forA.Key {
		t.Fatalf("first: %+v", first)
	}
	chat, _ := w.fake.Calls()
	var detected ask.Detected
	var evidence []ask.Item
	second, err := engine.Ask(context.Background(), ask.Request{Asker: w.asker(w.admin), Question: "And for Client B?", ThreadID: &first.ThreadID}, ask.Sink{
		Scope:    func(_ ask.Scope, d ask.Detected) { detected = d },
		Evidence: func(items []ask.Item) { evidence = items },
	})
	w.check(err)
	if !slices.Equal(detected.ClientIDs, []int64{w.b.ID}) || !slices.Equal(detected.NodeIDs, []int64{w.ot.ID}) {
		t.Fatalf("follow-up chips: %+v", detected)
	}
	for _, it := range evidence {
		if it.Key == forA.Key {
			t.Fatalf("Client A's ticket is evidence for Client B: %+v", evidence)
		}
	}
	if second.Status != ask.StatusAnswered || second.Claims[0].Cites[0] != forB.Key {
		t.Fatalf("second: %+v", second)
	}
	if after, _ := w.fake.Calls(); after != chat+1 {
		t.Fatalf("chat calls: %d then %d", chat, after)
	}
	if !strings.Contains(user, "CONVERSATION") || !strings.Contains(user, "Q: Why does overtime approval skip the supervisor for Client A?\nA: See the evidence. ["+forA.Key+"]") {
		t.Fatalf("prompt:\n%s", user)
	}
}
