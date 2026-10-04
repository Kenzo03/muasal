package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"fmt"
	"github.com/kenzo03/muasal/server/internal/auth"
	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/mail"
	"github.com/kenzo03/muasal/server/internal/testdb"
	"time"
)

const (
	origin = "http://muasal.test"
	pw     = "kopi-susu-di-kantor-7"
)

type env struct {
	t   *testing.T
	url string
	q   *db.Queries
	d   testdb.DB
	api *httpapi.Server
}

func newEnv(t *testing.T) *env { return newEnvWith(t, nil) }

// newEnvWith lets a test change the config, e.g. set APP_SECRET_KEY.
func newEnvWith(t *testing.T, change func(*config.Config)) *env {
	d := testdb.New(t)
	cfg := config.Config{
		DatabaseURL: d.AppURL, PublicURL: origin, ListenAddr: ":0",
		AttachmentsDir: t.TempDir(), AttachmentMaxBytes: 64 << 10,
	}
	if change != nil {
		change(&cfg)
	}
	api := httpapi.New(cfg, d.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(api.Close) // after the server closes: cleanups run last-in, first-out
	t.Cleanup(srv.Close)
	return &env{t: t, url: srv.URL, q: db.New(d.Pool), d: d, api: api}
}

// client is a separate browser with its own cookie jar.
func (e *env) client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

// call sends JSON from the app's origin and decodes the JSON reply into out when out is not nil.
func (e *env) call(c *http.Client, method, path string, body, out any) int {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.url+"/api/v1"+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

// seedUser creates a user who already has a password.
func (e *env) seedUser(email, password string, admin bool) db.User {
	e.t.Helper()
	ctx := context.Background()
	u, err := e.q.CreateUser(ctx, db.CreateUserParams{Email: email, Name: email, IsAdmin: admin, Locale: "id", Timezone: "Asia/Jakarta"})
	if err != nil {
		e.t.Fatal(err)
	}
	h := auth.HashPassword(password)
	if err := e.q.SetPasswordHash(ctx, db.SetPasswordHashParams{ID: u.ID, PasswordHash: &h}); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func login(e *env, c *http.Client, email, password string) (int, httpapi.Problem) {
	var p httpapi.Problem
	code := e.call(c, http.MethodPost, "/auth/login", map[string]string{"email": email, "password": password}, &p)
	return code, p
}

func TestLoginStartsASessionAndIsAudited(t *testing.T) {
	e := newEnv(t)
	u := e.seedUser("budi@example.com", pw, false)
	c := e.client()
	if code, _ := login(e, c, "Budi@Example.com", pw); code != http.StatusOK {
		t.Fatalf("login status %d", code)
	}
	var me httpapi.User
	if code := e.call(c, http.MethodGet, "/me", nil, &me); code != http.StatusOK || me.Email != "budi@example.com" || me.LastLoginAt == nil {
		t.Fatalf("me: %d %+v", code, me)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "user", EntityID: u.ID})
	if err != nil || len(events) != 1 || events[0].Action != "login" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestSessionCookieFlags(t *testing.T) {
	e := newEnv(t)
	e.seedUser("rina@example.com", pw, false)
	b, _ := json.Marshal(map[string]string{"email": "rina@example.com", "password": pw})
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/auth/login", bytes.NewReader(b))
	req.Header.Set("Origin", origin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var sid *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "sid" {
			sid = c
		}
	}
	if sid == nil || !sid.HttpOnly || sid.SameSite != http.SameSiteLaxMode || len(sid.Value) != 43 {
		t.Fatalf("bad session cookie: %+v", sid)
	}
}

func TestWrongPasswordIsRejected(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	code, p := login(e, e.client(), "budi@example.com", "wrong-password-123")
	if code != http.StatusUnauthorized || p.Code != "invalid_credentials" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

// AC-AD-2: after five wrong passwords, even the right one is refused,
// with the same answer as a wrong password.
func TestFiveFailuresLockTheAccount(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	for i := 1; i <= 5; i++ {
		if code, _ := login(e, c, "budi@example.com", "wrong-password-123"); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d", i, code)
		}
	}
	if code, p := login(e, c, "budi@example.com", pw); code != http.StatusUnauthorized || p.Code != "invalid_credentials" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

// Disabled and password-less accounts get the same answer as a wrong password.
func TestDisabledAndPasswordlessLoginsAreInvalidCredentials(t *testing.T) {
	e := newEnv(t)
	u := e.seedUser("budi@example.com", pw, false)
	e.exec(`UPDATE users SET disabled_at = now() WHERE id = $1`, u.ID)
	if code, p := login(e, e.client(), "budi@example.com", pw); code != http.StatusUnauthorized || p.Code != "invalid_credentials" {
		t.Fatalf("disabled: %d %s", code, p.Code)
	}
	if _, err := e.q.CreateUser(context.Background(), db.CreateUserParams{Email: "rina@example.com", Name: "Rina", Locale: "id", Timezone: "Asia/Jakarta"}); err != nil {
		t.Fatal(err)
	}
	// the password behind the stand-in hash must not open an account that has none
	if code, p := login(e, e.client(), "rina@example.com", "muasal-timing-equalizer"); code != http.StatusUnauthorized || p.Code != "invalid_credentials" {
		t.Fatalf("no password: %d %s", code, p.Code)
	}
}

func TestTwentyOneAttemptsFromOneIPAreRateLimited(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	for i := 0; i < 20; i++ {
		login(e, c, "nobody@example.com", "whatever-password")
	}
	if code, p := login(e, c, "nobody@example.com", "whatever-password"); code != http.StatusTooManyRequests || p.Code != "rate_limited" {
		t.Fatalf("got %d %s", code, p.Code)
	}
}

func TestWritesNeedTheAppOrigin(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/auth/login", strings.NewReader(`{"email":"a@b.c","password":"x"}`))
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestLogoutRevokesTheSession(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	u, _ := url.Parse(e.url)
	stolen := c.Jar.Cookies(u)
	if code := e.call(c, http.MethodPost, "/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout %d", code)
	}
	replay := e.client()
	replay.Jar.SetCookies(u, stolen)
	if code := e.call(replay, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("the old cookie still works: %d", code)
	}
}

func TestIdleSessionExpires(t *testing.T) {
	e := newEnv(t)
	e.seedUser("budi@example.com", pw, false)
	c := e.client()
	login(e, c, "budi@example.com", pw)
	if _, err := e.d.Pool.Exec(context.Background(), `UPDATE sessions SET last_seen_at = now() - interval '13 hours'`); err != nil {
		t.Fatal(err)
	}
	if code := e.call(c, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("got %d", code)
	}
}

func TestHealthChecks(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/healthz", "/readyz"} {
		res, err := http.Get(e.url + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("%s: %d", path, res.StatusCode)
		}
	}
}

// MSL-50: with email set up, a setup link also goes to its user by email, in
// their language; the response says where, and the link stays to copy.
func TestSetupLinksAreEmailed(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) {
		c.SMTP = mail.Config{Host: "smtp.example.test", Port: 587, From: "muasal@example.test", TLS: "starttls"}
	})
	sent := make(chan [3]string, 2)
	e.api.SetSendMail(func(_ mail.Config, to, subject, body string) error {
		sent <- [3]string{to, subject, body}
		return nil
	})
	admin, _ := e.signedIn("admin@example.com", true)
	var out httpapi.CreatedUser
	if code := e.call(admin, http.MethodPost, "/admin/users", map[string]any{"email": "budi@example.com", "name": "Budi Santoso", "locale": "id"}, &out); code != http.StatusCreated ||
		out.SetupLink.EmailedTo == nil || *out.SetupLink.EmailedTo != "budi@example.com" || out.SetupLink.Url == "" {
		t.Fatalf("create: %d %+v", code, out.SetupLink)
	}
	select {
	case m := <-sent:
		if m[0] != "budi@example.com" || m[1] != "Undangan ke Muasal" || !strings.Contains(m[2], "Halo Budi") || !strings.Contains(m[2], out.SetupLink.Url) {
			t.Fatalf("email: %q", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no email was sent")
	}
	var link httpapi.SetupLink
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/admin/users/%d/setup-link", out.User.Id), nil, &link); code != http.StatusCreated || link.EmailedTo == nil {
		t.Fatalf("new link: %d %+v", code, link)
	}
	select {
	case m := <-sent:
		if !strings.Contains(m[2], link.Url) {
			t.Fatalf("new link email: %q", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no email for the new link")
	}
}
