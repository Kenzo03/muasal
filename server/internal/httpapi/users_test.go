package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func setupToken(t *testing.T, link string) string {
	t.Helper()
	i := strings.LastIndex(link, "/setup/")
	if i < 0 {
		t.Fatalf("not a setup link: %q", link)
	}
	return link[i+len("/setup/"):]
}

// The Iteration 0 exit check at API level.
func TestAdminCreatesAUserWhoSetsAPasswordAndSignsIn(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	admin := e.client()
	if code, _ := login(e, admin, "admin@example.com", pw); code != http.StatusOK {
		t.Fatalf("admin login %d", code)
	}
	var created httpapi.CreatedUser
	code := e.call(admin, http.MethodPost, "/admin/users", map[string]any{"email": "budi@example.com", "name": "Budi"}, &created)
	if code != http.StatusCreated || created.User.HasPassword || !strings.HasPrefix(created.SetupLink.Url, origin+"/setup/") {
		t.Fatalf("create: %d %+v", code, created)
	}
	token := setupToken(t, created.SetupLink.Url)
	budi := e.client()
	var account struct{ Name, Email string } // MSL-20: the setup page names the account
	if code := e.call(budi, http.MethodPost, "/auth/setup/account", map[string]string{"token": token}, &account); code != http.StatusOK ||
		account.Name != "Budi" || account.Email != "budi@example.com" {
		t.Fatalf("setup account: %d %+v", code, account)
	}
	if code := e.call(budi, http.MethodPost, "/auth/setup", map[string]string{"token": token, "password": "nasi-goreng-pedas-99"}, nil); code != http.StatusNoContent {
		t.Fatalf("setup %d", code)
	}
	if code, _ := login(e, budi, "budi@example.com", "nasi-goreng-pedas-99"); code != http.StatusOK {
		t.Fatalf("budi login %d", code)
	}
	var p httpapi.Problem
	if code := e.call(budi, http.MethodPost, "/auth/setup", map[string]string{"token": token, "password": "nasi-goreng-pedas-99"}, &p); code != http.StatusGone || p.Code != "setup_link_invalid" {
		t.Fatalf("second use: %d %s", code, p.Code)
	}
	if code := e.call(budi, http.MethodPost, "/auth/setup/account", map[string]string{"token": token}, &p); code != http.StatusGone {
		t.Fatalf("a used link still names its account: %d", code)
	}
	var list httpapi.UserList
	if code := e.call(admin, http.MethodGet, "/admin/users", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("list: %d %+v", code, list)
	}
}

func TestDuplicateEmailIsRefused(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	admin := e.client()
	login(e, admin, "admin@example.com", pw)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/admin/users", map[string]any{"email": "ADMIN@example.com", "name": "Twin"}, &p); code != http.StatusConflict || p.Code != "email_taken" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

// AC-AD-3: a setup link older than 72 hours no longer works.
func TestExpiredSetupLinkIsRefused(t *testing.T) {
	e := newEnv(t)
	link, err := e.api.CreateAdmin(context.Background(), "admin@example.com", "Admin")
	if err != nil || !strings.HasPrefix(link, origin+"/setup/") {
		t.Fatalf("create admin: %q %v", link, err)
	}
	if _, err := e.d.Pool.Exec(context.Background(), `UPDATE setup_tokens SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	var p httpapi.Problem
	code := e.call(e.client(), http.MethodPost, "/auth/setup", map[string]string{"token": setupToken(t, link), "password": "nasi-goreng-pedas-99"}, &p)
	if code != http.StatusGone || p.Code != "setup_link_invalid" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

func TestSetupRejectsWeakPasswords(t *testing.T) {
	e := newEnv(t)
	link, err := e.api.CreateAdmin(context.Background(), "admin@example.com", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	for password, want := range map[string]string{"short": "password_too_short", "1qaz2wsx3edc": "password_too_common"} {
		var p httpapi.Problem
		code := e.call(e.client(), http.MethodPost, "/auth/setup", map[string]string{"token": setupToken(t, link), "password": password}, &p)
		if code != http.StatusUnprocessableEntity || p.Errors == nil || (*p.Errors)[0].Code != want {
			t.Errorf("%q: got %d %+v", password, code, p)
		}
	}
}

func TestNonAdminsCannotManageUsers(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	if code := e.call(c, http.MethodGet, "/admin/users", nil, nil); code != http.StatusForbidden {
		t.Fatalf("got %d", code)
	}
}

// AC-AD-1: disabling a user ends their session and blocks sign-in.
func TestDisablingAUserSignsThemOut(t *testing.T) {
	e := newEnv(t)
	adminUser := e.seedUser("admin@example.com", pw, true)
	budiUser := e.seedUser("budi@example.com", pw, false)
	admin, budi := e.client(), e.client()
	login(e, admin, "admin@example.com", pw)
	login(e, budi, "budi@example.com", pw)
	var updated httpapi.User
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/admin/users/%d", budiUser.ID), map[string]bool{"disabled": true}, &updated); code != http.StatusOK || !updated.Disabled {
		t.Fatalf("disable: %d %+v", code, updated)
	}
	if code := e.call(budi, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the disabled user is still signed in: %d", code)
	}
	if code, _ := login(e, budi, "budi@example.com", pw); code != http.StatusUnauthorized {
		t.Fatalf("the disabled user signed in again: %d", code)
	}
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/admin/users/%d", adminUser.ID), map[string]bool{"disabled": true}, &p); code != http.StatusUnprocessableEntity || p.Code != "cannot_change_self" {
		t.Fatalf("self-disable: %d %s", code, p.Code)
	}
}

func TestResetPasswordEndsSessionsAndIssuesANewLink(t *testing.T) {
	e := newEnv(t)
	e.seedUser("admin@example.com", pw, true)
	budiUser := e.seedUser("budi@example.com", pw, false)
	admin, budi := e.client(), e.client()
	login(e, admin, "admin@example.com", pw)
	login(e, budi, "budi@example.com", pw)
	var link httpapi.SetupLink
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/admin/users/%d/setup-link", budiUser.ID), nil, &link); code != http.StatusCreated || link.Url == "" {
		t.Fatalf("reset: %d %+v", code, link)
	}
	if code := e.call(budi, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("session survived the reset: %d", code)
	}
	if code, _ := login(e, e.client(), "budi@example.com", pw); code != http.StatusUnauthorized {
		t.Fatalf("the old password still works: %d", code)
	}
}

func TestChangingPasswordNeedsTheCurrentOne(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	var p httpapi.Problem
	if code := e.call(c, http.MethodPatch, "/me", map[string]string{"current_password": "not-the-password", "new_password": "teh-manis-dingin-42"}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong current password: %d", code)
	}
	var me httpapi.User
	if code := e.call(c, http.MethodPatch, "/me", map[string]string{"current_password": pw, "new_password": "teh-manis-dingin-42", "locale": "en"}, &me); code != http.StatusOK || me.Locale != "en" {
		t.Fatalf("change: %d %+v", code, me)
	}
	if code := e.call(c, http.MethodGet, "/me", nil, nil); code != http.StatusOK {
		t.Fatalf("the current session must survive: %d", code)
	}
	if code, _ := login(e, e.client(), "budi@example.com", "teh-manis-dingin-42"); code != http.StatusOK {
		t.Fatalf("the new password is rejected: %d", code)
	}
}
