package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/llm"
	"github.com/kenzo03/muasal/server/internal/secret"
)

// GetAISettings shows Admin → AI (FSD §13.4). Keys are never returned (R-AI-3).
func (s *Server) GetAISettings(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	cur, err := s.ai.Store.Get(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPIAISettings(cur))
}

// UpdateAISettings saves Admin → AI. BYOK needs the acknowledgement (R-AI-1);
// the save and the acknowledgement are audited, keys never.
func (s *Server) UpdateAISettings(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	var in AISettingsUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	cur, err := s.ai.Store.Get(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	next, fields := s.aiSettingsFrom(cur, in)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := s.ai.Store.Put(ctx, q, next, u.ID); err != nil {
			return err
		}
		m := webMeta(r)
		if d := changed(aiAudit(cur), aiAudit(next)); len(d) > 0 {
			if err := audit(ctx, q, m, &u.ID, "ai_settings", 1, "update", d); err != nil {
				return err
			}
		}
		if next.Mode == ai.ModeBYOK && (!cur.Acknowledged || cur.Provider != next.Provider || cur.Mode != ai.ModeBYOK) {
			return audit(ctx, q, m, &u.ID, "ai_settings", 1, "byok_acknowledged", map[string]any{"provider": next.Provider})
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPIAISettings(next))
}

// TestAI tries unsaved settings: the chat server's models, a one-sentence
// chat and one embedding, with latencies (§13.4). It saves nothing.
func (s *Server) TestAI(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	var in AISettingsUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	cur, err := s.ai.Store.Get(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	next, fields := s.aiSettingsFrom(cur, in)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	if next.Mode == ai.ModeOff {
		next.Mode = ai.ModeLocal // Off still lets the admin try a server before switching to it
	}
	chat, err := s.ai.ChatClient(next)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	embed, err := s.ai.EmbedClient(next)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, AITestResult{Chat: probeChat(r.Context(), chat), Embed: probeEmbed(r.Context(), embed)})
}

func probeChat(ctx context.Context, c *llm.Client) AIProbe {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	models, err := c.Models(ctx)
	if err == nil {
		var body io.ReadCloser
		body, err = c.ChatStream(ctx, llm.ChatRequest{
			System: "Reply with JSON only.", User: `Reply with {"ok": true}.`, MaxTokens: 20,
			Schema: json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`),
		})
		if err == nil {
			_, err = io.ReadAll(body)
			body.Close()
		}
	}
	p := AIProbe{Ok: err == nil, LatencyMs: int(time.Since(start).Milliseconds())}
	if models != nil {
		p.Models = &models
	}
	if err != nil {
		p.Error = ptr(err.Error())
	}
	return p
}

func probeEmbed(ctx context.Context, c *llm.Client) AIProbe {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	vecs, err := c.Embed(ctx, []string{"Muasal connection test"})
	p := AIProbe{Ok: err == nil, LatencyMs: int(time.Since(start).Milliseconds())}
	if err != nil {
		p.Error = ptr(err.Error())
	} else {
		p.Dim = ptr(len(vecs[0]))
	}
	return p
}

// aiSettingsFrom applies an update to the current settings: keys left out stay,
// an empty key is removed, a new key is sealed with APP_SECRET_KEY.
func (s *Server) aiSettingsFrom(cur ai.Settings, in AISettingsUpdate) (ai.Settings, []FieldError) {
	next := cur
	next.Mode, next.Provider, next.Acknowledged = ai.Mode(in.Mode), deref(in.Provider), deref(in.Acknowledged)
	next.ContextTokens, next.MaxConcurrent = in.Tuning.ContextTokens, in.Tuning.MaxConcurrent
	next.Temperature, next.TimeoutSeconds = in.Tuning.Temperature, in.Tuning.TimeoutSeconds
	next.MinSimilarity, next.ExhaustiveMax = in.Tuning.MinSimilarity, in.Tuning.ExhaustiveMax
	var fields []FieldError
	for _, e := range []struct {
		name string
		dst  *ai.Endpoint
		in   AIEndpointUpdate
	}{{"chat", &next.Chat, in.Chat}, {"embed", &next.Embed, in.Embed}} {
		e.dst.URL, e.dst.Model = e.in.Url, e.in.Model
		switch {
		case e.in.ApiKey == nil:
		case *e.in.ApiKey == "":
			e.dst.SealedKey = nil
		default:
			sealed, err := secret.Seal(s.cfg.SecretKey, []byte(*e.in.ApiKey))
			if errors.Is(err, secret.ErrNoKey) {
				fields = append(fields, FieldError{Field: e.name + ".api_key", Code: "secret_key_missing", Message: "Set APP_SECRET_KEY on the server before saving an API key"})
				continue
			}
			if err != nil {
				fields = append(fields, FieldError{Field: e.name + ".api_key", Code: "invalid", Message: err.Error()})
				continue
			}
			e.dst.SealedKey = sealed
		}
	}
	for _, p := range next.Validate() {
		fields = append(fields, FieldError{Field: p.Field, Code: p.Code, Message: p.Message})
	}
	return next, fields
}

func (s *Server) toAPIAISettings(c ai.Settings) AISettings {
	return AISettings{
		Mode: AIMode(c.Mode), Provider: c.Provider, Acknowledged: c.Acknowledged, EmbedDim: c.EmbedDim, Badge: c.Badge(),
		Chat:  AIEndpoint{Url: c.Chat.URL, Model: c.Chat.Model, ApiKeySet: len(c.Chat.SealedKey) > 0},
		Embed: AIEndpoint{Url: c.Embed.URL, Model: c.Embed.Model, ApiKeySet: len(c.Embed.SealedKey) > 0},
		Tuning: AITuning{
			ContextTokens: c.ContextTokens, MaxConcurrent: c.MaxConcurrent, Temperature: c.Temperature,
			TimeoutSeconds: c.TimeoutSeconds, MinSimilarity: c.MinSimilarity, ExhaustiveMax: c.ExhaustiveMax,
		},
		SecretKeySet: len(s.cfg.SecretKey) > 0,
	}
}

// aiAudit is what the history keeps of the settings: never a key, only
// whether one is saved (R-AI-3).
func aiAudit(c ai.Settings) map[string]any {
	return map[string]any{
		"mode": string(c.Mode), "provider": c.Provider, "acknowledged": c.Acknowledged,
		"chat_url": c.Chat.URL, "chat_model": c.Chat.Model, "chat_key": len(c.Chat.SealedKey) > 0,
		"embed_url": c.Embed.URL, "embed_model": c.Embed.Model, "embed_key": len(c.Embed.SealedKey) > 0,
		"context_tokens": c.ContextTokens, "max_concurrent": c.MaxConcurrent, "temperature": c.Temperature,
		"timeout_seconds": c.TimeoutSeconds, "min_similarity": c.MinSimilarity, "exhaustive_max": c.ExhaustiveMax,
	}
}
