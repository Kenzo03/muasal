package ai_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
	"github.com/kenzo03/muasal/server/internal/secret"
)

// R-AI-3: a sealed key is opened only to build the client; Off builds none.
func TestRuntimeBuildsClientsFromTheSettings(t *testing.T) {
	fake := llmtest.New(t)
	fake.Key = "sk-cloud"
	key := bytes.Repeat([]byte{3}, 32)
	sealed, _ := secret.Seal(key, []byte("sk-cloud"))
	rt := &ai.Runtime{SecretKey: key}
	s := ai.Defaults()
	if _, err := rt.ChatClient(s); !errors.Is(err, ai.ErrOff) {
		t.Fatalf("off: %v", err)
	}
	s.Mode = ai.ModeBYOK
	s.Chat = ai.Endpoint{URL: fake.BaseURL(), Model: "gpt-5-mini", SealedKey: sealed}
	c, err := rt.ChatClient(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Models(context.Background()); err != nil {
		t.Fatalf("the opened key must reach the provider: %v", err)
	}
}
