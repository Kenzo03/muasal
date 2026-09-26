package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
)

// Ask answers a question, streamed as SSE or as one JSON result by Accept
// (FSD §10, §11.6).
func (s *Server) Ask(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	if !s.askRate.Allow(strconv.FormatInt(u.ID, 10)) {
		writeProblem(w, http.StatusTooManyRequests, "rate_limited", "Ask takes 10 questions a minute; wait a moment")
		return
	}
	var in AskRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	question := strings.TrimSpace(in.Question)
	if n := len([]rune(question)); n == 0 || n > 1000 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "question", Code: "invalid", Message: "Ask a question of 1 to 1,000 characters"})
		return
	}
	ctx := r.Context()
	if in.ThreadId != nil {
		if _, err := s.q.GetThread(ctx, db.GetThreadParams{ID: *in.ThreadId, UserID: u.ID}); errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusNotFound, "not_found", "Thread not found")
			return
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	tz, err := time.LoadLocation(u.Timezone)
	if err != nil {
		tz = time.UTC
	}
	req := ask.Request{
		Asker:    ask.Asker{UserID: u.ID, IsAdmin: u.IsAdmin, Locale: u.Locale, TZ: tz},
		Question: question, Explicit: scopeFrom(in.Scope), ThreadID: in.ThreadId,
	}
	if in.Ignore != nil {
		for _, ig := range *in.Ignore {
			req.Ignore = append(req.Ignore, ask.Label{Kind: string(ig.Kind), ID: deref(ig.Id)})
		}
	}
	if in.Language != nil && *in.Language != AskRequestLanguageAuto {
		req.Language = string(*in.Language)
	}

	out := AskResult{Evidence: []AskItem{}, Claims: []AskClaim{}, Closest: []AskItem{}, Results: []AskItem{}}
	stream := strings.Contains(r.Header.Get("Accept"), "text/event-stream")
	var send func(event string, data any)
	if stream {
		var stop func()
		send, stop = startSSE(w)
		defer stop()
	}
	sink := ask.Sink{
		Queued: func(n int) {
			if send != nil {
				send("queued", map[string]int{"position": n})
			}
		},
		Scope: func(explicit ask.Scope, d ask.Detected) {
			out.Scope = AskScopeEvent{Explicit: toAPIScope(explicit), Detected: toAPIDetected(d)}
			if send != nil {
				send("scope", out.Scope)
			}
		},
		Evidence: func(items []ask.Item) {
			out.Evidence = toAPIItems(items)
			if send != nil {
				send("evidence", out.Evidence)
			}
		},
		Claim: func(c ask.Claim) {
			claim := AskClaim{Text: c.Text, Cites: c.Cites}
			out.Claims = append(out.Claims, claim)
			if send != nil {
				send("claim", claim)
			}
		},
	}
	res, err := s.engine.Ask(ctx, req, sink)
	if err != nil {
		s.log.Error("ask failed", "request_id", requestIDFrom(ctx), "err", err)
		if send != nil {
			send("error", map[string]string{"code": "internal", "message": "Something went wrong"})
			return
		}
		s.fail(w, r, err)
		return
	}
	out.Status = AskResultStatus(res.Status)
	out.QueryId, out.ThreadId, out.Language = res.QueryID, res.ThreadID, AskResultLanguage(res.Language)
	out.Closest, out.Results = toAPIItems(res.Closest), toAPIItems(res.Results)
	if res.Model != "" {
		out.Model = &res.Model
	}
	switch res.Status {
	case ask.StatusNotEnough:
		out.Message = ptr(ask.NotEnough)
	case ask.StatusError:
		out.ErrorCode = ptr(AskResultErrorCode(res.ErrorCode))
		out.Message = ptr(askErrors[res.ErrorCode])
	}
	if send == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	if res.Status == ask.StatusError {
		send("error", map[string]string{"code": res.ErrorCode, "message": askErrors[res.ErrorCode]})
	}
	send("result", map[string]any{
		"status": out.Status, "query_id": out.QueryId, "thread_id": out.ThreadId, "language": out.Language,
		"model": out.Model, "message": out.Message, "closest": out.Closest, "results": out.Results,
	})
}

var askErrors = map[string]string{
	"ai_unavailable": "AI server unavailable",
	"ai_busy":        "AI server busy",
	"ai_timeout":     "The AI server did not respond in time",
	"ai_invalid":     "The AI server did not respond in time",
}

// startSSE switches the response to an event stream. send writes one event;
// a comment every 15 seconds keeps proxies from closing an idle stream (§11.6).
func startSSE(w http.ResponseWriter) (send func(event string, data any), stop func()) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var mu sync.Mutex
	write := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprint(w, s)
		if flusher != nil {
			flusher.Flush()
		}
	}
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				write(": keep-alive\n\n")
			case <-done:
				return
			}
		}
	}()
	send = func(event string, data any) {
		b, _ := json.Marshal(data)
		write("event: " + event + "\ndata: " + string(b) + "\n\n")
	}
	return send, func() { close(done) }
}

// ListAskThreads lists the asker's own threads (§10.6).
func (s *Server) ListAskThreads(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListThreads(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := AskThreadList{Items: make([]AskThread, len(rows))}
	for i, t := range rows {
		out.Items[i] = AskThread{Id: t.ID, Title: t.Title, CreatedAt: t.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetAskThread reads one of the asker's threads; anyone else's is not found.
func (s *Server) GetAskThread(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	t, err := s.q.GetThread(ctx, db.GetThreadParams{ID: id, UserID: u.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows, err := s.q.ListThreadQueries(ctx, &t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := AskThreadDetail{Id: t.ID, Title: t.Title, CreatedAt: t.CreatedAt, Queries: make([]AskThreadQuery, len(rows))}
	for i, q := range rows {
		claims := []AskClaim{}
		if q.Answer != nil {
			if err := json.Unmarshal(q.Answer, &claims); err != nil {
				s.fail(w, r, err)
				return
			}
		}
		var logged []loggedEvidence
		if err := json.Unmarshal(q.Evidence, &logged); err != nil {
			s.fail(w, r, err)
			return
		}
		ids := make([]int64, len(logged))
		for j, l := range logged {
			ids[j] = l.TicketID
		}
		items, err := s.engine.ItemsFor(ctx, &ask.Asker{UserID: u.ID, IsAdmin: u.IsAdmin}, ids)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		evidence := toAPIItems(items)
		out.Queries[i] = AskThreadQuery{Id: q.ID, Question: q.Question, Status: q.Status, Claims: claims, Model: q.Model, CreatedAt: q.CreatedAt, Evidence: &evidence}
	}
	writeJSON(w, http.StatusOK, out)
}

// HideAskThread takes a thread off the asker's list; the log keeps it (§10.6).
func (s *Server) HideAskThread(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	n, err := s.q.HideThread(r.Context(), db.HideThreadParams{ID: id, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if n == 0 {
		writeProblem(w, http.StatusNotFound, "not_found", "Thread not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func scopeFrom(in *AskScope) ask.Scope {
	if in == nil {
		return ask.Scope{}
	}
	s := ask.Scope{
		ProjectIDs: deref(in.ProjectIds), NodeIDs: deref(in.NodeIds), ClientIDs: deref(in.ClientIds),
		UserIDs: deref(in.UserIds), ContactIDs: deref(in.ContactIds),
	}
	if in.From != nil {
		s.From = &in.From.Time
	}
	if in.To != nil {
		s.To = &in.To.Time
	}
	return s
}

func toAPIScope(s ask.Scope) AskScope {
	out := AskScope{}
	if len(s.ProjectIDs) > 0 {
		out.ProjectIds = &s.ProjectIDs
	}
	if len(s.NodeIDs) > 0 {
		out.NodeIds = &s.NodeIDs
	}
	if len(s.ClientIDs) > 0 {
		out.ClientIds = &s.ClientIDs
	}
	if len(s.UserIDs) > 0 {
		out.UserIds = &s.UserIDs
	}
	if len(s.ContactIDs) > 0 {
		out.ContactIds = &s.ContactIDs
	}
	if s.From != nil {
		out.From = &openapi_types.Date{Time: *s.From}
	}
	if s.To != nil {
		out.To = &openapi_types.Date{Time: *s.To}
	}
	return out
}

func toAPIDetected(d ask.Detected) AskDetected {
	sc := toAPIScope(ask.Scope{NodeIDs: d.NodeIDs, ClientIDs: d.ClientIDs, UserIDs: d.UserIDs, ContactIDs: d.ContactIDs, From: d.From, To: d.To})
	out := AskDetected{NodeIds: sc.NodeIds, ClientIds: sc.ClientIds, UserIds: sc.UserIds, ContactIds: sc.ContactIds, From: sc.From, To: sc.To}
	if len(d.Keys) > 0 {
		out.Keys = &d.Keys
	}
	if len(d.Labels) > 0 {
		labels := make([]AskLabel, len(d.Labels))
		for i, l := range d.Labels {
			labels[i] = AskLabel{Kind: AskLabelKind(l.Kind), Id: l.ID, Label: l.Label}
		}
		out.Labels = &labels
	}
	return out
}

func toAPIItems(items []ask.Item) []AskItem {
	out := make([]AskItem, len(items))
	for i, it := range items {
		out[i] = AskItem{Key: it.Key, Title: it.Title, Client: it.Client, RequestedBy: it.RequestedBy, Date: it.Date, Status: it.Status, Closed: it.Closed}
	}
	return out
}
