package ask

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/llm"
)

// NotEnough is the one wording for an answer without evidence (§10.4). It
// never hints that hidden tickets exist.
const NotEnough = "Not enough information in the tickets you can access to answer this."

// Statuses of an answer, as the Ask log stores them.
const (
	StatusAnswered  = "answered"
	StatusNotEnough = "not_enough_info"
	StatusAIOff     = "ai_off"
	StatusError     = "error"
)

// Item is one ticket as sources, citation chips and the closest list show it,
// read from the database, so names and dates are exact (§10.3).
type Item struct {
	Key         string    `json:"key"`
	Title       string    `json:"title"`
	Client      *string   `json:"client"`
	RequestedBy string    `json:"requested_by"`
	Date        time.Time `json:"date"` // closed date, or created date while open
	Status      string    `json:"status"`
	Closed      bool      `json:"closed"`
}

// Request is one question. Explicit is the scope the page or the user set.
type Request struct {
	Asker    Asker
	Question string
	Explicit Scope
	Language string // "", "id" or "en"; "" detects it (§10.5)
	ThreadID *int64
}

// Sink receives the answer as it forms; the HTTP handler turns each call into
// an SSE event (§11.6). Any field may be nil.
type Sink struct {
	Queued   func(ahead int)
	Scope    func(explicit Scope, detected Detected)
	Evidence func([]Item)
	Claim    func(Claim)
}

// Result is the end of an answer.
type Result struct {
	Status    string
	QueryID   int64
	ThreadID  int64
	Language  string
	Model     string // the badge, e.g. "Local · qwen3.5:4b"
	Claims    []Claim
	Closest   []Item // with not enough information
	Results   []Item // AI off: keyword results under the same scope
	ErrorCode string // ai_unavailable, ai_busy, ai_timeout, ai_invalid
}

// Engine answers questions. Seed fixes sampling for evaluation (§11.5).
type Engine struct {
	pool *pgxpool.Pool
	q    *db.Queries
	ai   *ai.Runtime
	now  func() time.Time
	Seed *int
}

// NewEngine returns an engine over pool that calls models as rt's settings say.
func NewEngine(pool *pgxpool.Pool, rt *ai.Runtime) *Engine {
	return &Engine{pool: pool, q: db.New(pool), ai: rt, now: time.Now}
}

// QueueWait is how long a question waits for a model slot before "AI server
// busy" (§11.7).
var QueueWait = 90 * time.Second

// Ask answers one question: scope, evidence, constrained claims, validation
// and the log row (§11). It returns an error only when it could not log; model
// failures end in a Result with status error.
func (e *Engine) Ask(ctx context.Context, r Request, sink Sink) (Result, error) {
	start := e.now()
	s, err := e.ai.Store.Get(ctx)
	if err != nil {
		return Result{}, err
	}
	tz := r.Asker.TZ
	if tz == nil {
		tz = time.UTC
	}
	cat, err := LoadCatalog(ctx, e.q, r.Asker)
	if err != nil {
		return Result{}, err
	}
	detected := Detect(cat, r.Question, start.In(tz))
	scope := Merge(r.Explicit, detected)
	if sink.Scope != nil {
		sink.Scope(r.Explicit, detected)
	}
	res := Result{Language: r.Language}
	if res.Language == "" {
		res.Language = Language(r.Question, r.Asker.Locale)
	}
	logRow := logEntry{scope: map[string]any{"explicit": r.Explicit, "detected": detected}}
	tuning := Tuning{MinSimilarity: s.MinSimilarity, ExhaustiveMax: s.ExhaustiveMax}

	// Off: keyword search under the same scope, no model call (§13.4, AC-IX-5).
	if s.Mode == ai.ModeOff {
		found, err := Retrieve(ctx, e.pool, r.Asker, scope, r.Question, nil, detected.Keys, tuning)
		if err != nil {
			return Result{}, err
		}
		if res.Results, err = e.items(ctx, found.TicketIDs, 12); err != nil {
			return Result{}, err
		}
		res.Status = StatusAIOff
		return e.finish(ctx, r, res, logRow, start)
	}

	res.Model = s.Badge()
	fail := func(code string) (Result, error) {
		res.Status, res.ErrorCode = StatusError, code
		logRow.err = code
		return e.finish(ctx, r, res, logRow, start)
	}
	embed, err := e.ai.EmbedClient(s)
	if err != nil {
		return fail("ai_unavailable")
	}
	vecs, err := embed.Embed(ctx, []string{r.Question})
	if err != nil {
		return fail("ai_unavailable")
	}
	tuning.EmbedModel = s.Embed.Model
	found, err := Retrieve(ctx, e.pool, r.Asker, scope, r.Question, vecs[0], detected.Keys, tuning)
	if err != nil {
		return Result{}, err
	}
	logRow.evidence = found
	if len(found.TicketIDs) == 0 {
		// Nothing in scope passes the floor: the chat model is not called (AC-AK-5).
		if res.Closest, err = e.items(ctx, found.Closest, 5); err != nil {
			return Result{}, err
		}
		res.Status = StatusNotEnough
		return e.finish(ctx, r, res, logRow, start)
	}
	sources := make([]indexer.Source, 0, len(found.TicketIDs))
	for _, id := range found.TicketIDs {
		src, err := indexer.Load(ctx, e.q, id)
		if err != nil {
			return Result{}, err
		}
		sources = append(sources, src)
	}
	text, keys := Pack(sources, s.ContextTokens)
	evidence := make([]Item, 0, len(keys))
	for _, src := range sources[:len(keys)] {
		evidence = append(evidence, itemOf(src))
	}
	if sink.Evidence != nil {
		sink.Evidence(evidence)
	}

	chat, err := e.ai.ChatClient(s)
	if err != nil {
		return fail("ai_unavailable")
	}
	waitCtx, cancelWait := context.WithTimeout(ctx, QueueWait)
	release, err := e.ai.Gate.Acquire(waitCtx, s.MaxConcurrent, sink.Queued)
	cancelWait()
	if err != nil {
		return fail("ai_busy")
	}
	logRow.llmCalled = true
	genCtx, cancel := context.WithTimeout(ctx, time.Duration(s.TimeoutSeconds)*time.Second)
	body, err := chat.ChatStream(genCtx, llm.ChatRequest{
		System: System(res.Language), User: User(r.Question, text), Schema: Schema(keys),
		Temperature: s.Temperature, MaxTokens: 600, Seed: e.Seed,
	})
	if err == nil {
		err = Claims(body, func(c Claim) {
			valid, dropped := Validate(c, keys)
			if dropped != nil {
				logRow.dropped = append(logRow.dropped, *dropped)
			}
			if valid.Text == "" || len(res.Claims) >= 6 {
				return
			}
			if len(res.Claims) == 0 {
				logRow.firstClaim = e.now().Sub(start)
			}
			res.Claims = append(res.Claims, valid)
			if sink.Claim != nil {
				sink.Claim(valid)
			}
		})
		body.Close()
	}
	timedOut := genCtx.Err() != nil
	cancel()
	release()
	switch {
	case timedOut:
		return fail("ai_timeout")
	case err != nil && len(res.Claims) == 0:
		var apiErr *llm.APIError
		if errors.As(err, &apiErr) {
			return fail("ai_unavailable")
		}
		return fail("ai_invalid")
	case len(res.Claims) == 0:
		// The server decides the status: no surviving claim is not enough
		// information, with the closest tickets (§11.5).
		res.Status, res.Closest = StatusNotEnough, evidence[:min(5, len(evidence))]
	default:
		res.Status = StatusAnswered
	}
	return e.finish(ctx, r, res, logRow, start)
}

type logEntry struct {
	scope      map[string]any
	evidence   Found
	llmCalled  bool
	dropped    []Dropped
	firstClaim time.Duration
	err        string
}

// finish writes the Ask log row, and the thread for a first question (§10.8).
func (e *Engine) finish(ctx context.Context, r Request, res Result, l logEntry, start time.Time) (Result, error) {
	threadID := r.ThreadID
	if threadID == nil {
		title := strings.TrimSpace(r.Question)
		if runes := []rune(title); len(runes) > 80 {
			title = string(runes[:80]) + "…"
		}
		th, err := e.q.CreateThread(ctx, db.CreateThreadParams{UserID: r.Asker.UserID, Title: title})
		if err != nil {
			return res, err
		}
		threadID = &th.ID
	}
	res.ThreadID = *threadID
	evidence := make([]map[string]any, 0, len(l.evidence.TicketIDs))
	for _, id := range l.evidence.TicketIDs {
		evidence = append(evidence, map[string]any{"ticket_id": id, "score": l.evidence.Scores[id]})
	}
	scope, _ := json.Marshal(l.scope)
	ev, _ := json.Marshal(evidence)
	var answer, dropped []byte
	if res.Claims != nil {
		answer, _ = json.Marshal(res.Claims)
	}
	if l.dropped != nil || l.err != "" {
		dropped, _ = json.Marshal(map[string]any{"claims": l.dropped, "error": l.err})
	}
	p := db.InsertAskQueryParams{
		ThreadID: threadID, UserID: r.Asker.UserID, Question: r.Question, Lang: res.Language, Scope: scope, Evidence: ev,
		LlmCalled: l.llmCalled, Status: res.Status, Answer: answer, Dropped: dropped,
		LatencyMs: ptrTo(int32(e.now().Sub(start).Milliseconds())),
	}
	if res.Model != "" {
		p.Model = &res.Model
	}
	if l.firstClaim > 0 {
		p.FirstClaimMs = ptrTo(int32(l.firstClaim.Milliseconds()))
	}
	id, err := e.q.InsertAskQuery(ctx, p)
	res.QueryID = id
	return res, err
}

// items reads up to n tickets as Items, in order.
func (e *Engine) items(ctx context.Context, ids []int64, n int) ([]Item, error) {
	out := []Item{}
	for _, id := range ids[:min(n, len(ids))] {
		src, err := indexer.Load(ctx, e.q, id)
		if err != nil {
			return nil, err
		}
		out = append(out, itemOf(src))
	}
	return out, nil
}

// ItemsFor reads the given tickets as Items, in order. With an asker, only
// those they may open now are kept (§10.6); without one, all of them (the Ask
// log, for system admins).
func (e *Engine) ItemsFor(ctx context.Context, a *Asker, ids []int64) ([]Item, error) {
	if a != nil && len(ids) > 0 {
		ok, err := e.q.VisibleTicketIDs(ctx, db.VisibleTicketIDsParams{Ids: ids, IsAdmin: a.IsAdmin, UserID: a.UserID})
		if err != nil {
			return nil, err
		}
		ids = slices.DeleteFunc(slices.Clone(ids), func(id int64) bool { return !slices.Contains(ok, id) })
	}
	return e.items(ctx, ids, len(ids))
}

func itemOf(src indexer.Source) Item {
	t := src.Ticket
	it := Item{Key: t.Key, Title: t.Title, Client: t.ClientName, RequestedBy: indexer.Requester(t), Date: t.CreatedAt, Status: t.StatusName}
	if t.ClosedAt != nil {
		it.Date, it.Closed = *t.ClosedAt, true
	}
	return it
}

func ptrTo[T any](v T) *T { return &v }
