package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/auth"
	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/db"
)

// Server implements the generated ServerInterface.
type Server struct {
	cfg     config.Config
	pool    *pgxpool.Pool
	q       *db.Queries
	ipLimit *auth.Limiter
	log     *slog.Logger
	now     func() time.Time
	jobs    *river.Client[pgx.Tx] // inserts jobs only; `serve` runs the workers (FSD §13.2)
	ai      *ai.Runtime
	engine  *ask.Engine
	askRate *auth.Limiter
}

// New wires a Server; it opens no connections of its own.
func New(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Server {
	jobs, err := river.NewClient(riverpgxv5.New(pool), &river.Config{Logger: log})
	if err != nil {
		panic(err) // an insert-only client with a fixed config cannot fail
	}
	q := db.New(pool)
	s := &Server{
		cfg:     cfg,
		pool:    pool,
		q:       q,
		ipLimit: auth.NewLimiter(20, time.Minute), // FSD §15.1: 20 sign-in attempts per IP per minute
		log:     log,
		now:     time.Now,
		jobs:    jobs,
		ai:      &ai.Runtime{Store: ai.NewStore(q), Gate: ai.NewGate(), SecretKey: cfg.SecretKey, HTTP: &http.Client{}},
		askRate: auth.NewLimiter(10, time.Minute), // FSD §17.1: Ask 10 a minute per user
	}
	s.engine = ask.NewEngine(pool, s.ai)
	return s
}

// AI is the runtime the API shares with the index workers in the same process:
// one settings cache and one generation gate (§11.7).
func (s *Server) AI() *ai.Runtime { return s.ai }

// Handler serves the API under /api/v1 plus unauthenticated health checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.readyz)
	mux.HandleFunc("GET /metrics", s.metrics)
	HandlerWithOptions(s, StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
		// The last middleware runs first: the Origin check, then the session lookup.
		Middlewares: []MiddlewareFunc{s.authenticate, s.requireOrigin},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		},
	})
	return securityHeaders(s.requestContext(mux))
}

// securityHeaders sets FSD §18.2's headers on every response. The API serves
// data, never pages, so its policy allows nothing to run or frame it.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "database_unavailable", "The database is not reachable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// inTx runs fn in one transaction, so a change and its audit event commit together (FSD §4.2).
func (s *Server) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	return s.inJobTx(ctx, func(q *db.Queries, _ pgx.Tx) error { return fn(q) })
}

// inJobTx is inTx for changes that also queue jobs: fn gets the transaction
// for River's InsertTx, so a job commits or rolls back with its change (§4.2).
func (s *Server) inJobTx(ctx context.Context, fn func(q *db.Queries, tx pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	if err := fn(s.q.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditMeta records where a change came from.
type auditMeta struct {
	via       string
	requestID *string
	ip        *netip.Addr
	projectID *int64 // lets project admins search their project's history (FSD §5.1)
}

// inProject tags the event with its project.
func (m auditMeta) inProject(id int64) auditMeta {
	m.projectID = &id
	return m
}

var systemMeta = auditMeta{via: "system"}

func webMeta(r *http.Request) auditMeta {
	return auditMeta{via: "web", requestID: ptr(requestIDFrom(r.Context())), ip: ipAddr(r)}
}

// audit appends one event. changes must never contain secrets.
func audit(ctx context.Context, q *db.Queries, m auditMeta, actorID *int64, entity string, entityID int64, action string, changes any) error {
	if changes == nil {
		changes = map[string]any{}
	}
	b, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	return q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ActorID: actorID, Via: m.via, Entity: entity, EntityID: entityID, ProjectID: m.projectID,
		Action: action, Changes: b, RequestID: m.requestID, Ip: m.ip,
	})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "request_id", requestIDFrom(r.Context()), "err", err)
	writeProblem(w, http.StatusInternalServerError, "internal", "Something went wrong")
}

func ptr[T any](v T) *T { return &v }

// changed keeps the fields whose values differ, as {"field": {"old": …, "new": …}} (FSD §16).
func changed(before, after map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range after {
		if !reflect.DeepEqual(before[k], v) {
			out[k] = map[string]any{"old": before[k], "new": v}
		}
	}
	return out
}

// constraintOf names the database constraint err violated, or "".
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

func deref[T any](p *T) (v T) {
	if p != nil {
		v = *p
	}
	return v
}

// orEmpty keeps a JSON array [] instead of null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
