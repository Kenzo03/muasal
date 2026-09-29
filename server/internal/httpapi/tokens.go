package httpapi

import (
	"crypto/rand"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/auth"
	"github.com/kenzo03/muasal/server/internal/db"
)

// ListTokens lists the caller's live API tokens (FSD §14.3).
func (s *Server) ListTokens(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListAPITokens(r.Context(), u.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := APITokenList{Items: make([]APIToken, len(rows))}
	for i, t := range rows {
		out.Items[i] = toAPIToken(t)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateToken makes a personal access token: msl_ and 32 random bytes. The
// secret leaves the server once, in this response.
func (s *Server) CreateToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.sessionOnly(w, r)
	if !ok {
		return
	}
	var in APITokenCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 100 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "name", Code: "invalid", Message: "Use 1 to 100 characters"})
		return
	}
	var expires *time.Time
	if in.ExpiresOn != nil {
		// The token works through the whole of its last day, in the owner's timezone.
		tz := userTZ(u)
		d := in.ExpiresOn.Time
		end := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, tz).AddDate(0, 0, 1)
		if !end.After(s.now()) {
			writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
				FieldError{Field: "expires_on", Code: "invalid", Message: "Choose today or a later day"})
			return
		}
		expires = &end
	}
	secret := tokenPrefix + rand.Text() + rand.Text() // 52 base32 characters, 260 bits
	ctx := r.Context()
	var t db.ApiToken
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if t, err = q.CreateAPIToken(ctx, db.CreateAPITokenParams{
			UserID: u.ID, Name: name, TokenHash: auth.HashToken(secret), ReadOnly: in.ReadOnly, ExpiresAt: expires,
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "token", t.ID, "create", map[string]any{"name": name, "read_only": in.ReadOnly})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a := toAPIToken(t)
	writeJSON(w, http.StatusCreated, APITokenCreated{
		Id: a.Id, Name: a.Name, ReadOnly: a.ReadOnly, ExpiresAt: a.ExpiresAt, LastUsedAt: a.LastUsedAt, CreatedAt: a.CreatedAt, Token: secret,
	})
}

// RevokeToken ends one of the caller's tokens at once.
func (s *Server) RevokeToken(w http.ResponseWriter, r *http.Request, id int64) {
	u, ok := s.sessionOnly(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		t, err := q.RevokeAPIToken(ctx, db.RevokeAPITokenParams{ID: id, UserID: u.ID})
		if err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "token", t.ID, "revoke", map[string]any{"name": t.Name})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Token not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sessionOnly requires a signed-in browser session: a leaked token must not
// mint or revoke tokens.
func (s *Server) sessionOnly(w http.ResponseWriter, r *http.Request) (*db.User, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return nil, false
	}
	if currentToken(r) != nil {
		writeProblem(w, http.StatusForbidden, "session_required", "Manage API tokens from a signed-in session")
		return nil, false
	}
	return u, true
}

func toAPIToken(t db.ApiToken) APIToken {
	return APIToken{Id: t.ID, Name: t.Name, ReadOnly: t.ReadOnly, ExpiresAt: t.ExpiresAt, LastUsedAt: t.LastUsedAt, CreatedAt: t.CreatedAt}
}
