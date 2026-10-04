package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/auth"
	"github.com/kenzo03/muasal/server/internal/db"
)

// dummyHash makes sign-in for an unknown email as slow as for a known one.
var dummyHash = auth.HashPassword("muasal-timing-equalizer")

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var in LoginRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if !s.ipLimit.Allow(clientIP(r)) {
		writeProblem(w, http.StatusTooManyRequests, "rate_limited", "Too many sign-in attempts from this address; wait a minute")
		return
	}
	ctx := r.Context()
	u, err := s.q.GetUserByEmail(ctx, strings.TrimSpace(in.Email))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.fail(w, r, err)
		return
	}
	// Every outcome below runs exactly one password check and refuses with the same answer.
	pwHash := dummyHash
	if err == nil && u.PasswordHash != nil {
		pwHash = *u.PasswordHash
	}
	passwordOK := auth.CheckPassword(pwHash, in.Password) && err == nil && u.PasswordHash != nil
	if err != nil || (u.LockedUntil != nil && u.LockedUntil.After(s.now())) {
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is wrong")
		return
	}
	if u.DisabledAt != nil || !passwordOK {
		err := s.inTx(ctx, func(q *db.Queries) error {
			if err := q.RecordLoginFailure(ctx, u.ID); err != nil {
				return err
			}
			return audit(ctx, q, webMeta(r), nil, "user", u.ID, "login_failed", nil)
		})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Email or password is wrong")
		return
	}
	token, hash := auth.NewToken()
	expires := s.now().Add(absoluteTimeout)
	err = s.inTx(ctx, func(q *db.Queries) error {
		if old, err := r.Cookie(sessionCookie); err == nil { // the new session replaces this browser's old one
			if err := q.DeleteSession(ctx, auth.HashToken(old.Value)); err != nil {
				return err
			}
		}
		if err := q.RecordLoginSuccess(ctx, u.ID); err != nil {
			return err
		}
		if err := q.CreateSession(ctx, db.CreateSessionParams{
			TokenHash: hash, UserID: u.ID, ExpiresAt: expires, Ip: ipAddr(r), UserAgent: ptr(r.UserAgent()),
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "login", nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.setSessionCookie(w, token, expires)
	u.LastLoginAt = ptr(s.now())
	writeJSON(w, http.StatusOK, toAPIUser(u))
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if hash := currentSessionHash(r); hash != nil {
		u := currentUser(r)
		ctx := r.Context()
		err := s.inTx(ctx, func(q *db.Queries) error {
			if err := q.DeleteSession(ctx, hash); err != nil {
				return err
			}
			return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "logout", nil)
		})
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.setSessionCookie(w, "", time.Unix(0, 0))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetMe(w http.ResponseWriter, r *http.Request) {
	if u := s.requireUser(w, r); u != nil {
		writeJSON(w, http.StatusOK, toAPIUser(*u))
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.cfg.SecureCookies(), SameSite: http.SameSiteLaxMode,
	})
}

func toAPIUser(u db.User) User {
	return User{
		Id: u.ID, Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin,
		Locale: Locale(u.Locale), Timezone: u.Timezone,
		Disabled: u.DisabledAt != nil, HasPassword: u.PasswordHash != nil,
		CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt, NotifyPrefs: notifyPrefs(u),
	}
}
