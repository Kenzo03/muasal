package httpapi_test

import (
	"bytes"
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
)

// aiUpdate is a complete Admin → AI form for mode against one fake server.
func aiUpdate(mode, url string) map[string]any {
	return map[string]any{
		"mode":  mode,
		"chat":  map[string]any{"url": url, "model": "qwen3.5:4b"},
		"embed": map[string]any{"url": url, "model": "bge-m3"},
		"tuning": map[string]any{"context_tokens": 2500, "max_concurrent": 1, "temperature": 0.1,
			"timeout_seconds": 60, "min_similarity": 0.45, "exhaustive_max": 40},
	}
}

func withSecretKey(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{9}, 32) }

// §13.4: a fresh install is Off; only system admins read or change the settings.
func TestAISettingsAreForSystemAdmins(t *testing.T) {
	e := newEnvWith(t, withSecretKey)
	admin, _ := e.signedIn("admin@example.com", true)
	member, _ := e.signedIn("rina@example.com", false)
	var got httpapi.AISettings
	if code := e.call(admin, http.MethodGet, "/admin/settings/ai", nil, &got); code != http.StatusOK ||
		got.Mode != httpapi.AIModeOff || !got.SecretKeySet || got.Badge != "Local · qwen3.5:4b" {
		t.Fatalf("fresh install: %d %+v", code, got)
	}
	if code := e.call(member, http.MethodGet, "/admin/settings/ai", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member reads the settings: %d", code)
	}
	if code := e.call(member, http.MethodPut, "/admin/settings/ai", aiUpdate("off", "http://model:11434/v1"), nil); code != http.StatusForbidden {
		t.Fatalf("a member saves the settings: %d", code)
	}
}

// AC-IX-6 and R-AI-1: BYOK without the acknowledgement is refused and nothing
// reaches the provider; with it, the key is sealed, never returned, and the
// acknowledgement is audited.
func TestBYOKNeedsTheAcknowledgementAndHidesTheKey(t *testing.T) {
	e := newEnvWith(t, withSecretKey)
	admin, _ := e.signedIn("admin@example.com", true)
	provider := llmtest.New(t)
	body := aiUpdate("byok", provider.BaseURL())
	body["provider"] = "Example Cloud"
	body["chat"].(map[string]any)["api_key"] = "sk-secret-123"
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "byok_not_acknowledged" {
		t.Fatalf("BYOK without the acknowledgement: %d %+v", code, p)
	}
	if code := e.call(admin, http.MethodPost, "/admin/ai/test", body, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("testing BYOK without the acknowledgement: %d", code)
	}
	if chat, embed := provider.Calls(); chat+embed != 0 || len(provider.Embedded) != 0 {
		t.Fatalf("the provider was called: chat %d, embed %d", chat, embed)
	}

	body["acknowledged"] = true
	var got httpapi.AISettings
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, &got); code != http.StatusOK ||
		!got.Chat.ApiKeySet || got.Embed.ApiKeySet || got.Badge != "Cloud · Example Cloud · qwen3.5:4b" {
		t.Fatalf("BYOK with the acknowledgement: %d %+v", code, got)
	}
	delete(body["chat"].(map[string]any), "api_key") // left out: the saved key stays
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, &got); code != http.StatusOK || !got.Chat.ApiKeySet {
		t.Fatalf("a save without the key field: %d %+v", code, got)
	}

	ctx := context.Background()
	var actions []string
	var leaked bool
	rows, err := e.d.Pool.Query(ctx, "SELECT action, changes::text FROM audit_events WHERE entity = 'ai_settings' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var action, changes string
		if err := rows.Scan(&action, &changes); err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
		leaked = leaked || strings.Contains(changes, "sk-secret")
	}
	if !slices.Equal(actions, []string{"update", "byok_acknowledged"}) || leaked {
		t.Fatalf("audit: %v, key leaked: %v", actions, leaked)
	}
	var stored string
	if err := e.d.Pool.QueryRow(ctx, "SELECT value::text FROM settings WHERE key = 'ai'").Scan(&stored); err != nil || strings.Contains(stored, "sk-secret") {
		t.Fatalf("the stored settings hold the key in the clear: %v", err)
	}
}

// R-AI-3: without APP_SECRET_KEY a key cannot be saved.
func TestKeysNeedTheSecretKey(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	body := aiUpdate("local", "http://model:11434/v1")
	body["chat"].(map[string]any)["api_key"] = "sk-vllm"
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPut, "/admin/settings/ai", body, &p); code != http.StatusUnprocessableEntity ||
		firstError(p).Field != "chat.api_key" || firstError(p).Code != "secret_key_missing" {
		t.Fatalf("a key without APP_SECRET_KEY: %d %+v", code, p)
	}
}

// §13.4: Test connection reports models, the chat and the embedding dimension,
// and a stopped server as a failed probe.
func TestConnectionTestReportsEachEndpoint(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	fake := llmtest.New(t)
	var res httpapi.AITestResult
	if code := e.call(admin, http.MethodPost, "/admin/ai/test", aiUpdate("local", fake.BaseURL()), &res); code != http.StatusOK ||
		!res.Chat.Ok || res.Chat.Models == nil || len(*res.Chat.Models) != 2 || !res.Embed.Ok || res.Embed.Dim == nil || *res.Embed.Dim != 1024 {
		t.Fatalf("a running server: %d %+v", code, res)
	}
	fake.Set(func(s *llmtest.Server) { s.Down = true })
	if code := e.call(admin, http.MethodPost, "/admin/ai/test", aiUpdate("local", fake.BaseURL()), &res); code != http.StatusOK ||
		res.Chat.Ok || res.Chat.Error == nil || res.Embed.Ok {
		t.Fatalf("a stopped server: %d %+v", code, res)
	}
	var got httpapi.AISettings
	e.call(admin, http.MethodGet, "/admin/settings/ai", nil, &got)
	if got.Mode != httpapi.AIModeOff {
		t.Fatalf("a test must not save: %+v", got)
	}
}
