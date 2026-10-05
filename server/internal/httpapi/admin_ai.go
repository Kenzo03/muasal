package httpapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/ai"
	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/indexer"
	"github.com/kenzo03/zettra/server/internal/llm"
	"github.com/kenzo03/zettra/server/internal/secret"
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
	// A new embedding model re-embeds every chunk, so the admin confirms it
	// (§13.4). A new URL for the same model changes nothing in the index.
	embedChanged := next.Embed.Model != cur.Embed.Model || next.EmbedDim != cur.EmbedDim
	if embedChanged && !deref(in.Reindex) {
		chunks, err := s.q.CountEmbeddedChunks(ctx)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if chunks > 0 {
			fields = append(fields, FieldError{Field: "embed.model", Code: "reindex_required",
				Message: fmt.Sprintf("A new embedding model re-embeds all %d embedded chunks; confirm to continue", chunks)})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := s.ai.Store.Put(ctx, q, next, u.ID); err != nil {
			return err
		}
		switch {
		case next.EmbedDim != cur.EmbedDim:
			if _, err := s.jobs.InsertTx(ctx, tx, indexer.ChangeDimension{Dim: next.EmbedDim}, nil); err != nil {
				return err
			}
		case embedChanged || (cur.Mode == ai.ModeOff && next.Mode != ai.ModeOff):
			// Chunks written while AI was off, or embedded by the old model, get vectors now (R-AI-5).
			if _, err := s.jobs.InsertTx(ctx, tx, indexer.EmbedPending{}, nil); err != nil {
				return err
			}
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
	if in.Mode == AIModeOff {
		in.Mode = AIModeLocal // Off still lets the admin try a server before switching to it, checked as Local
	}
	next, fields := s.aiSettingsFrom(cur, in)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
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
		p.Reason = probeReason(err)
	}
	return p
}

func probeEmbed(ctx context.Context, c *llm.Client) AIProbe {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	start := time.Now()
	vecs, err := c.Embed(ctx, []string{"Zettra connection test"})
	p := AIProbe{Ok: err == nil, LatencyMs: int(time.Since(start).Milliseconds())}
	if err != nil {
		p.Reason = probeReason(err)
	} else {
		p.Dim = ptr(len(vecs[0]))
	}
	return p
}

// probeReason names a failed probe's likely cause (MSL-31), or nil when there
// is none to name. The server's own answer is never passed on.
func probeReason(err error) *AIProbeReason {
	var dns *net.DNSError
	var cert *tls.CertificateVerificationError
	var timeout net.Error
	var apiErr *llm.APIError
	r := AIProbeReason("")
	switch {
	case errors.As(err, &dns):
		r = AIProbeReasonUnknownHost
	case errors.Is(err, syscall.ECONNREFUSED):
		r = AIProbeReasonRefused
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout():
		r = AIProbeReasonTimeout
	case errors.As(err, &cert):
		r = AIProbeReasonTls
	case !errors.As(err, &apiErr):
		return nil
	case apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden:
		r = AIProbeReasonUnauthorized
	case apiErr.Status == http.StatusNotFound && strings.Contains(strings.ToLower(apiErr.Body), "model"):
		r = AIProbeReasonNoModel // Ollama, vLLM and OpenAI all name the model they lack
	case apiErr.Status == http.StatusNotFound:
		r = AIProbeReasonNotFound
	default:
		return nil
	}
	return &r
}

// aiSettingsFrom applies an update to the current settings: keys left out stay
// unless the URL's scheme, host or port changes, an empty key is removed, a new
// key is sealed with APP_SECRET_KEY.
func (s *Server) aiSettingsFrom(cur ai.Settings, in AISettingsUpdate) (ai.Settings, []FieldError) {
	next := cur
	next.Mode, next.Provider, next.Acknowledged = ai.Mode(in.Mode), deref(in.Provider), deref(in.Acknowledged)
	next.ContextTokens, next.MaxConcurrent = in.Tuning.ContextTokens, in.Tuning.MaxConcurrent
	next.Temperature, next.TimeoutSeconds = in.Tuning.Temperature, in.Tuning.TimeoutSeconds
	next.MinSimilarity, next.ExhaustiveMax = in.Tuning.MinSimilarity, in.Tuning.ExhaustiveMax
	if in.EmbedDim != nil {
		next.EmbedDim = *in.EmbedDim
	}
	var fields []FieldError
	for _, e := range []struct {
		name string
		dst  *ai.Endpoint
		in   AIEndpointUpdate
	}{{"chat", &next.Chat, in.Chat}, {"embed", &next.Embed, in.Embed}} {
		moved := !ai.SameServer(e.dst.URL, e.in.Url)
		e.dst.URL, e.dst.Model = e.in.Url, e.in.Model
		switch {
		case e.in.ApiKey == nil && moved:
			e.dst.SealedKey = nil // a saved key goes only to the server it was saved for
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

// GetAIStatus is Index status (§13.3).
func (s *Server) GetAIStatus(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	ctx := r.Context()
	cur, err := s.ai.Store.Get(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := indexer.ReadStatus(ctx, s.pool, cur.Embed.Model)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := IndexStatus{
		Mode: AIMode(cur.Mode), EmbedModel: cur.Embed.Model, TotalChunks: st.Total, PendingChunks: st.Pending,
		QueuedJobs: st.Queued, LastIndexedAt: st.LastIndexedAt,
		ChunksByModel: make([]ModelChunks, len(st.ByModel)), FailedJobs: make([]FailedJob, len(st.Failed)),
	}
	for i, m := range st.ByModel {
		out.ChunksByModel[i] = ModelChunks{Model: m.Model, Chunks: m.Chunks}
	}
	for i, f := range st.Failed {
		out.FailedJobs[i] = FailedJob{Id: f.ID, TicketId: f.TicketID, Attempts: f.Attempts, Error: f.Error, At: f.At}
	}
	writeJSON(w, http.StatusOK, out)
}

// ReindexAI queues every ticket, or retries the index jobs that failed for
// good (§13.2); the admin sees the queue drain in Index status.
func (s *Server) ReindexAI(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	var in ReindexRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	n := 0
	var err error
	switch in.Scope {
	case ReindexRequestScopeAll:
		n, err = indexer.QueueAll(ctx, s.pool, s.jobs)
	case ReindexRequestScopeFailed:
		var st indexer.Status
		if st, err = indexer.ReadStatus(ctx, s.pool, ""); err == nil {
			for _, f := range st.Failed {
				if _, err = s.jobs.JobRetry(ctx, f.ID); err != nil {
					break
				}
				n++
			}
		}
	default:
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "scope", Code: "invalid", Message: "Choose all or failed"})
		return
	}
	if err == nil {
		err = audit(ctx, s.q, webMeta(r), &u.ID, "ai_settings", 1, "reindex", map[string]any{"scope": string(in.Scope), "queued": n})
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, ReindexResult{Queued: n})
}
