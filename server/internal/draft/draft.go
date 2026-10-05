// Package draft writes with the chat model for people to check: decision
// records drafted from a ticket's thread (FSD §9.3) and change summaries
// (§12.1). Nothing it returns is saved until a person saves it.
package draft

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/kenzo03/zettra/server/internal/ai"
	"github.com/kenzo03/zettra/server/internal/llm"
)

// Error is a model failure the API answers with its code: ai_off,
// ai_unavailable, ai_busy, ai_timeout or ai_invalid.
type Error struct{ Code string }

func (e *Error) Error() string { return "draft: " + e.Code }

// QueueWait bounds the wait for a generation slot, shared with Ask (§9.3).
var QueueWait = 90 * time.Second

// generate runs one constrained chat call under the Ask concurrency limit and
// decodes its JSON answer into out.
func generate(ctx context.Context, rt *ai.Runtime, s ai.Settings, r llm.ChatRequest, out any) error {
	if s.Mode == ai.ModeOff {
		return &Error{"ai_off"}
	}
	chat, err := rt.ChatClient(s)
	if err != nil {
		return &Error{"ai_unavailable"}
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, QueueWait)
	release, err := rt.Gate.Acquire(waitCtx, s.MaxConcurrent, nil)
	cancelWait()
	if err != nil {
		return &Error{"ai_busy"}
	}
	defer release()
	genCtx, cancel := context.WithTimeout(ctx, time.Duration(s.TimeoutSeconds)*time.Second)
	defer cancel()
	if r.Temperature == 0 {
		r.Temperature = s.Temperature
	}
	body, err := chat.ChatStream(genCtx, r)
	var text []byte
	if err == nil {
		text, err = io.ReadAll(body)
		body.Close()
	}
	switch {
	case genCtx.Err() != nil:
		return &Error{"ai_timeout"}
	case err != nil:
		var apiErr *llm.APIError
		if errors.As(err, &apiErr) {
			return &Error{"ai_unavailable"}
		}
		return &Error{"ai_invalid"}
	}
	if err := json.Unmarshal(text, out); err != nil {
		return &Error{"ai_invalid"}
	}
	return nil
}

func languageName(lang string) string {
	if lang == "en" {
		return "English"
	}
	return "Bahasa Indonesia"
}

// clip cuts s to n runes, trimmed.
func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}
