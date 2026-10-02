package llm_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/llm"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
)

func TestModelsAndEmbeddings(t *testing.T) {
	fake := llmtest.New(t)
	fake.Key = "sk-local"
	c := llm.New(fake.BaseURL(), "bge-m3", "sk-local", nil)
	ctx := context.Background()
	models, err := c.Models(ctx)
	if err != nil || len(models) != 2 || models[1] != "bge-m3" {
		t.Fatalf("models: %v %v", models, err)
	}
	vecs, err := c.Embed(ctx, []string{"overtime approval", "leave balance"})
	if err != nil || len(vecs) != 2 || len(vecs[0]) != 1024 {
		t.Fatalf("embed: %d %v", len(vecs), err)
	}
	if _, err := llm.New(fake.BaseURL(), "bge-m3", "wrong", nil).Models(ctx); err == nil {
		t.Fatal("a wrong key must fail")
	}
}

// §11.5: the stream's content deltas arrive as one text, however the server
// splits them; the request carries the schema and skips the thinking pass.
func TestChatStreamJoinsTheDeltas(t *testing.T) {
	fake := llmtest.New(t)
	answer := `{"claims":[{"text":"Overtime skips the supervisor for Client A.","cites":["HRIS-231"]}]}`
	fake.Answer = func(system, user string, schema json.RawMessage) string { return answer }
	seed := 7
	r, err := llm.New(fake.BaseURL(), "qwen3.5:4b", "", nil).ChatStream(context.Background(), llm.ChatRequest{
		System: "Answer only from EVIDENCE.", User: "Why?", Schema: json.RawMessage(`{"type":"object"}`),
		Temperature: 0.1, MaxTokens: 600, Seed: &seed,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != answer {
		t.Fatalf("text: %q %v", got, err)
	}
	body := fake.LastChat
	rf := body["response_format"].(map[string]any)
	if body["reasoning_effort"] != "none" || rf["type"] != "json_schema" || body["seed"] != float64(7) || body["max_tokens"] != float64(600) {
		t.Fatalf("request: %v", body)
	}
}

// §11.5: a provider without JSON-schema output falls back to plain JSON mode.
func TestChatFallsBackToJSONMode(t *testing.T) {
	fake := llmtest.New(t)
	fake.RejectSchema = true
	fake.Answer = func(string, string, json.RawMessage) string { return `{"claims":[]}` }
	r, err := llm.New(fake.BaseURL(), "m", "", nil).ChatStream(context.Background(), llm.ChatRequest{Schema: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(r)
	if rf := fake.LastChat["response_format"].(map[string]any); rf["type"] != "json_object" || fake.LastChat["reasoning_effort"] != nil {
		t.Fatalf("fallback request: %v", fake.LastChat)
	}
}

// §11.7: a 429 retries with backoff inside the caller's deadline; other errors do not.
func TestRateLimitsRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"data":[{"id":"m"}]}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if m, err := llm.New(srv.URL, "m", "", nil).Models(ctx); err != nil || len(m) != 1 || calls.Load() != 2 {
		t.Fatalf("after a 429: %v %v %d", m, err, calls.Load())
	}
	fake := llmtest.New(t)
	fake.Down = true
	var apiErr *llm.APIError
	if _, err := llm.New(fake.BaseURL(), "m", "", nil).Embed(context.Background(), []string{"x"}); !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Fatalf("a stopped server: %v", err)
	}
}

// A redirect is not followed, so the key goes only to the base URL it was given.
func TestRedirectsAreNotFollowed(t *testing.T) {
	var auth atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
	}))
	defer other.Close()
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusFound)
	}))
	defer moved.Close()
	for _, hc := range []*http.Client{nil, {}} {
		_, err := llm.New(moved.URL+"/v1", "bge-m3", "sk-local", hc).Models(context.Background())
		var apiErr *llm.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusFound || auth.Load() != nil {
			t.Fatalf("a redirect: %v, the other host saw %v", err, auth.Load())
		}
	}
}

// An error names the status, never the server's own words; APIError.Body keeps
// them for the server's use.
func TestErrorsLeaveOutTheServersAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			http.Error(w, "SECRET", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"error\":{\"message\":\"SECRET\"}}\n\n")
	}))
	defer srv.Close()
	c := llm.New(srv.URL+"/v1", "m", "", nil)
	_, err := c.Models(context.Background())
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) || apiErr.Body != "SECRET" || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("a 500: %v", err)
	}
	r, err := c.ChatStream(context.Background(), llm.ChatRequest{})
	if err == nil {
		_, err = io.ReadAll(r)
	}
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("an error in the stream: %v", err)
	}
}
