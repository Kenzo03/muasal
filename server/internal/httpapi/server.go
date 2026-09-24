package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

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
}

// New wires a Server; it opens no connections of its own.
func New(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		pool:    pool,
		q:       db.New(pool),
		ipLimit: auth.NewLimiter(20, time.Minute), // FSD §15.1: 20 sign-in attempts per IP per minute
		log:     log,
		now:     time.Now,
	}
}

// Handler serves the API under /api/v1 plus unauthenticated health checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", s.readyz)
	HandlerWithOptions(s, StdHTTPServerOptions{
		BaseURL:    "/api/v1",
		BaseRouter: mux,
		// The last middleware runs first: the Origin check, then the session lookup.
		Middlewares: []MiddlewareFunc{s.authenticate, s.requireOrigin},
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", err.Error())
		},
	})
	return s.requestContext(mux)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // no-op after Commit
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// auditMeta records where a change came from.
type auditMeta struct {
	via       string
	requestID *string
	ip        *netip.Addr
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
		ActorID: actorID, Via: m.via, Entity: entity, EntityID: entityID,
		Action: action, Changes: b, RequestID: m.requestID, Ip: m.ip,
	})
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "request_id", requestIDFrom(r.Context()), "err", err)
	writeProblem(w, http.StatusInternalServerError, "internal", "Something went wrong")
}

func ptr[T any](v T) *T { return &v }
