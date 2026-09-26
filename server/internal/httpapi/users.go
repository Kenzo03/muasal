package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kenzo03/muasal/server/internal/auth"
	"github.com/kenzo03/muasal/server/internal/db"
)

const setupLinkTTL = 72 * time.Hour // FSD §15.1

func (s *Server) ListUsers(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	rows, err := s.q.ListUsers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]User, len(rows))
	for i, u := range rows {
		items[i] = toAPIUser(u)
	}
	writeJSON(w, http.StatusOK, UserList{Items: items})
}

func (s *Server) CreateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in UserCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	email := strings.TrimSpace(in.Email)
	fields := validateProfile(&in.Name, in.Locale, in.Timezone)
	if !validEmail(email) {
		fields = append(fields, FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	params := db.CreateUserParams{
		Email: email, Name: strings.TrimSpace(in.Name), IsAdmin: in.IsAdmin != nil && *in.IsAdmin,
		Locale: "id", Timezone: "Asia/Jakarta",
	}
	if in.Locale != nil {
		params.Locale = string(*in.Locale)
	}
	if in.Timezone != nil {
		params.Timezone = *in.Timezone
	}
	ctx := r.Context()
	var out CreatedUser
	err := s.inTx(ctx, func(q *db.Queries) error {
		u, err := q.CreateUser(ctx, params)
		if err != nil {
			return err
		}
		link, err := s.issueSetupLink(ctx, q, u.ID)
		if err != nil {
			return err
		}
		out = CreatedUser{User: toAPIUser(u), SetupLink: link}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", u.ID, "create",
			map[string]any{"email": params.Email, "name": params.Name, "is_admin": params.IsAdmin})
	})
	if isUniqueViolation(err) {
		writeProblem(w, http.StatusConflict, "email_taken", "A user with this email already exists",
			FieldError{Field: "email", Code: "email_taken", Message: "A user with this email already exists"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateUser(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in UserUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if id == admin.ID && ((in.Disabled != nil && *in.Disabled) || (in.IsAdmin != nil && !*in.IsAdmin)) {
		writeProblem(w, http.StatusUnprocessableEntity, "cannot_change_self", "You cannot disable yourself or remove your own admin role")
		return
	}
	if fields := validateProfile(in.Name, in.Locale, in.Timezone); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.User
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateUser(ctx, db.UpdateUserParams{
			ID: id, Name: trimmed(in.Name), IsAdmin: in.IsAdmin, Locale: localeString(in.Locale), Timezone: in.Timezone,
		}); err != nil {
			return err
		}
		if in.Disabled != nil {
			if updated, err = q.SetDisabled(ctx, db.SetDisabledParams{ID: id, Disabled: *in.Disabled}); err != nil {
				return err
			}
			if *in.Disabled { // AC-AD-1: a disabled user is signed out everywhere at once
				if err := q.DeleteUserSessions(ctx, id); err != nil {
					return err
				}
			}
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", id, "update", in)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated))
}

// CreateSetupLink resets a password: the old one stops working, every session
// ends, and a new one-time link is returned (FSD §15.1).
func (s *Server) CreateSetupLink(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	ctx := r.Context()
	var link SetupLink
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetUserByID(ctx, id); err != nil {
			return err
		}
		if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: id, PasswordHash: nil}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, id); err != nil {
			return err
		}
		var err error
		if link, err = s.issueSetupLink(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "user", id, "reset_password", nil)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// SetupPassword redeems a one-time setup link.
func (s *Server) SetupPassword(w http.ResponseWriter, r *http.Request) {
	var in SetupRequest
	if !decodeJSON(w, r, &in) {
		return
	}
	if err := auth.CheckPolicy(in.Password); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", passwordField("password", err))
		return
	}
	hash := auth.HashPassword(in.Password)
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		userID, err := q.UseSetupToken(ctx, auth.HashToken(in.Token))
		if err != nil {
			return err
		}
		if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: userID, PasswordHash: &hash}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &userID, "user", userID, "set_password", nil)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusGone, "setup_link_invalid", "This link has expired or was already used. Ask your admin for a new one.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateMe edits the signed-in user's own profile and password.
func (s *Server) UpdateMe(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in MeUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	fields := validateProfile(in.Name, in.Locale, in.Timezone)
	var newHash *string
	if in.NewPassword != nil {
		policyErr := auth.CheckPolicy(*in.NewPassword)
		switch {
		case in.CurrentPassword == nil || u.PasswordHash == nil || !auth.CheckPassword(*u.PasswordHash, *in.CurrentPassword):
			fields = append(fields, FieldError{Field: "current_password", Code: "wrong_password", Message: "The current password is wrong"})
		case policyErr != nil:
			fields = append(fields, passwordField("new_password", policyErr))
		default:
			newHash = ptr(auth.HashPassword(*in.NewPassword))
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.User
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateUser(ctx, db.UpdateUserParams{
			ID: u.ID, Name: trimmed(in.Name), Locale: localeString(in.Locale), Timezone: in.Timezone,
		}); err != nil {
			return err
		}
		if in.NotifyPrefs != nil {
			prefs, _ := json.Marshal(in.NotifyPrefs)
			if updated, err = q.SetNotifyPrefs(ctx, db.SetNotifyPrefsParams{ID: u.ID, Prefs: prefs}); err != nil {
				return err
			}
		}
		changes := map[string]any{"name": in.Name, "locale": in.Locale, "timezone": in.Timezone}
		if newHash != nil {
			if err := q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: u.ID, PasswordHash: newHash}); err != nil {
				return err
			}
			// Other devices sign out; this one stays signed in (FSD §18.2).
			if err := q.DeleteOtherSessions(ctx, db.DeleteOtherSessionsParams{UserID: u.ID, TokenHash: currentSessionHash(r)}); err != nil {
				return err
			}
			changes["password"] = "changed"
		}
		return audit(ctx, q, webMeta(r), &u.ID, "user", u.ID, "update_profile", changes)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIUser(updated))
}

// CreateAdmin makes an admin from the CLI and returns their setup link.
func (s *Server) CreateAdmin(ctx context.Context, email, name string) (string, error) {
	email, name = strings.TrimSpace(email), strings.TrimSpace(name)
	if !validEmail(email) || name == "" {
		return "", errors.New("a valid --email and a non-empty --name are required")
	}
	var link SetupLink
	err := s.inTx(ctx, func(q *db.Queries) error {
		u, err := q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: name, IsAdmin: true, Locale: "id", Timezone: "Asia/Jakarta"})
		if err != nil {
			return err
		}
		if link, err = s.issueSetupLink(ctx, q, u.ID); err != nil {
			return err
		}
		return audit(ctx, q, systemMeta, nil, "user", u.ID, "create", map[string]any{"email": email, "name": name, "is_admin": true})
	})
	if isUniqueViolation(err) {
		return "", fmt.Errorf("a user with email %s already exists", email)
	}
	return link.Url, err
}

// issueSetupLink voids the user's older links and returns a new one.
func (s *Server) issueSetupLink(ctx context.Context, q *db.Queries, userID int64) (SetupLink, error) {
	token, hash := auth.NewToken()
	expires := s.now().Add(setupLinkTTL)
	if err := q.VoidSetupTokens(ctx, userID); err != nil {
		return SetupLink{}, err
	}
	if err := q.CreateSetupToken(ctx, db.CreateSetupTokenParams{TokenHash: hash, UserID: userID, ExpiresAt: expires}); err != nil {
		return SetupLink{}, err
	}
	return SetupLink{Url: s.cfg.PublicURL + "/setup/" + token, ExpiresAt: expires}, nil
}

func validateProfile(name *string, locale *Locale, timezone *string) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if locale != nil && *locale != "id" && *locale != "en" {
		f = append(f, FieldError{Field: "locale", Code: "invalid", Message: "Choose id or en"})
	}
	if timezone != nil {
		if _, err := time.LoadLocation(*timezone); err != nil || *timezone == "" {
			f = append(f, FieldError{Field: "timezone", Code: "invalid", Message: "Unknown timezone"})
		}
	}
	return f
}

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && len(s) <= 320
}

func passwordField(field string, err error) FieldError {
	msg := "Use at least 12 characters"
	if errors.Is(err, auth.ErrPasswordTooCommon) {
		msg = "This password is too common; choose another"
	}
	return FieldError{Field: field, Code: err.Error(), Message: msg}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func trimmed(p *string) *string {
	if p == nil {
		return nil
	}
	return ptr(strings.TrimSpace(*p))
}

func localeString(l *Locale) *string {
	if l == nil {
		return nil
	}
	return ptr(string(*l))
}
