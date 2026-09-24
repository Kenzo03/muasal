package httpapi

import (
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/muasal/muasal/server/internal/auth"
	"github.com/muasal/muasal/server/internal/db"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	userKey
	sessionHashKey
)

const (
	sessionCookie   = "sid"
	idleTimeout     = 12 * time.Hour     // FSD §17.1
	absoluteTimeout = 7 * 24 * time.Hour // FSD §17.1
)

// statusWriter records the status code for the access log.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach Flush for streaming responses later.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// requestContext gives every request an ID, logs it, recovers panics and sets
// headers that every API response needs.
func (s *Server) requestContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-Id", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("panic", "request_id", id, "panic", p)
				writeProblem(sw, http.StatusInternalServerError, "internal", "Something went wrong")
			}
			s.log.Info("request", "request_id", id, "method", r.Method, "path", r.URL.Path,
				"status", sw.status, "ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// authenticate attaches the signed-in user when the sid cookie names a live
// session. Handlers decide whether a user is required.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sessionCookie); err == nil {
			hash := auth.HashToken(c.Value)
			if u := s.sessionUser(r.Context(), hash); u != nil {
				ctx := context.WithValue(r.Context(), userKey, u)
				r = r.WithContext(context.WithValue(ctx, sessionHashKey, hash))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// sessionUser returns the session's user, or nil when the session is unknown,
// idle for 12 hours, past its 7-day limit or owned by a disabled user.
func (s *Server) sessionUser(ctx context.Context, hash []byte) *db.User {
	row, err := s.q.GetSession(ctx, hash)
	if err != nil {
		return nil
	}
	now := s.now()
	if now.After(row.ExpiresAt) || now.Sub(row.LastSeenAt) > idleTimeout || row.User.DisabledAt != nil {
		_ = s.q.DeleteSession(ctx, hash)
		return nil
	}
	if now.Sub(row.LastSeenAt) > time.Minute { // ponytail: one write per session per minute, not per request
		_ = s.q.TouchSession(ctx, hash)
	}
	return &row.User
}

func currentUser(r *http.Request) *db.User {
	u, _ := r.Context().Value(userKey).(*db.User)
	return u
}

func currentSessionHash(r *http.Request) []byte {
	h, _ := r.Context().Value(sessionHashKey).([]byte)
	return h
}

// requireOrigin blocks cross-site writes (FSD §17.1): unsafe methods must come
// from PUBLIC_URL. Next.js server-side calls only read, so they never hit this.
func (s *Server) requireOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("Origin") != s.cfg.PublicURL {
				writeProblem(w, http.StatusForbidden, "bad_origin", "The request origin is not allowed")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP is the address Caddy saw. Caddy replaces X-Forwarded-For sent by
// untrusted clients, and the app is reachable only through Caddy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func ipAddr(r *http.Request) *netip.Addr {
	a, err := netip.ParseAddr(clientIP(r))
	if err != nil {
		return nil
	}
	return &a
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) *db.User {
	u := currentUser(r)
	if u == nil {
		writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in first")
	}
	return u
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) *db.User {
	u := s.requireUser(w, r)
	if u != nil && !u.IsAdmin {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only admins can do this")
		return nil
	}
	return u
}
