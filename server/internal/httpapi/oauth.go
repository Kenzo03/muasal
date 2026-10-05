package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/auth"
	"github.com/kenzo03/zettra/server/internal/db"
)

// MCP sign-in (MCP spec): Zettra is the authorization server for its own
// /mcp endpoint. Clients register themselves, a signed-in user approves them on
// /oauth/authorize, and the code they get back buys an ordinary API token.

// mcpResource is the protected resource the tokens are for.
func (s *Server) mcpResource() string { return s.cfg.PublicURL + "/mcp" }

// protectedResource is RFC 9728's metadata.
func (s *Server) protectedResource(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"resource":                 s.mcpResource(),
		"authorization_servers":    []string{s.cfg.PublicURL},
		"bearer_methods_supported": []string{"header"},
	})
}

// authServerMetadata is RFC 8414's metadata.
func (s *Server) authServerMetadata(w http.ResponseWriter, _ *http.Request) {
	u := s.cfg.PublicURL
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                u,
		"authorization_endpoint":                u + "/oauth/authorize",
		"token_endpoint":                        u + "/oauth/token",
		"registration_endpoint":                 u + "/oauth/register",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
	})
}

// writeOAuthError answers in RFC 6749's error format, which OAuth clients parse.
func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// validRedirectURI allows https anywhere and http only on the loopback
// address, where native clients listen (RFC 8252). No fragments.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || len(raw) > 2000 || strings.Contains(raw, "#") {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		h := u.Hostname()
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	return false
}

// registerClient is RFC 7591 dynamic registration for public clients.
// Registering grants nothing: a signed-in user still has to approve.
func (s *Server) registerClient(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimit.Allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests from this address; wait a minute")
		return
	}
	var in struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	// Clients send more metadata than this; unknown fields are ignored.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "Send the client metadata as JSON")
		return
	}
	name := strings.TrimSpace(in.ClientName)
	if name == "" {
		name = "MCP client"
	}
	if utf8.RuneCountInString(name) > 100 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client_name takes at most 100 characters")
		return
	}
	if len(in.RedirectURIs) < 1 || len(in.RedirectURIs) > 5 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Give 1 to 5 redirect URIs")
		return
	}
	for _, u := range in.RedirectURIs {
		if !validRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "Redirect URIs use https, or http on localhost, with no fragment or login, at most 2000 characters")
			return
		}
	}
	c, err := s.q.CreateOAuthClient(r.Context(), db.CreateOAuthClientParams{ID: rand.Text(), Name: name, RedirectUris: in.RedirectURIs})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectUris,
		"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code"}, "response_types": []string{"code"},
	})
}

// codeTTL is how long an approval's code may be exchanged.
const codeTTL = 10 * time.Minute

// GetOAuthClient shows the approval page which client is asking.
func (s *Server) GetOAuthClient(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.sessionOnly(w, r); !ok {
		return
	}
	c, err := s.q.GetOAuthClient(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "This app is not registered")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, OAuthClient{Id: c.ID, Name: c.Name, RedirectUris: c.RedirectUris})
}

// ApproveOAuth records the signed-in user's answer. It redirects only to a
// URI the client registered, so a bad request never leaves Zettra.
func (s *Server) ApproveOAuth(w http.ResponseWriter, r *http.Request) {
	u, ok := s.sessionOnly(w, r)
	if !ok {
		return
	}
	var in OAuthApprove
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	c, err := s.q.GetOAuthClient(ctx, in.ClientId)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.fail(w, r, err)
		return
	}
	invalid := func(msg string) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_authorization_request", msg)
	}
	switch {
	case err != nil:
		invalid("This app is not registered")
		return
	case !slices.Contains(c.RedirectUris, in.RedirectUri):
		invalid("The app asked to return to an address it did not register")
		return
	case in.CodeChallengeMethod != "S256" || len(in.CodeChallenge) < 43 || len(in.CodeChallenge) > 128:
		invalid("The app must use PKCE with S256")
		return
	case in.Resource != nil && *in.Resource != "" && *in.Resource != s.mcpResource():
		invalid("The app asked for access to another server")
		return
	}
	q := url.Values{}
	if in.State != nil && *in.State != "" {
		q.Set("state", *in.State)
	}
	action := "deny"
	if in.Allow {
		code, hash := auth.NewToken()
		if err := s.q.CreateOAuthCode(ctx, db.CreateOAuthCodeParams{
			CodeHash: hash, ClientID: c.ID, UserID: u.ID, RedirectUri: in.RedirectUri,
			CodeChallenge: in.CodeChallenge, ReadOnly: in.ReadOnly, ExpiresAt: s.now().Add(codeTTL),
		}); err != nil {
			s.fail(w, r, err)
			return
		}
		q.Set("code", code)
		action = "approve"
	} else {
		q.Set("error", "access_denied")
	}
	if err := audit(ctx, s.q, webMeta(r), &u.ID, "oauth_client", 0, action, map[string]any{"client_id": c.ID, "name": c.Name, "read_only": in.ReadOnly}); err != nil {
		s.fail(w, r, err)
		return
	}
	sep := "?"
	if strings.Contains(in.RedirectUri, "?") {
		sep = "&"
	}
	writeJSON(w, http.StatusOK, OAuthRedirect{RedirectUrl: in.RedirectUri + sep + encodeQuery(q)})
}

// encodeQuery keeps code (or error) before state, as clients log it.
func encodeQuery(q url.Values) string {
	var parts []string
	for _, k := range []string{"code", "error", "state"} {
		if v := q.Get(k); v != "" {
			parts = append(parts, k+"="+url.QueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

// exchangeCode is the token endpoint. The code is spent before anything is
// checked, so a wrong verifier can't be retried against the same code.
func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request) {
	if !s.ipLimit.Allow(clientIP(r)) {
		writeOAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many requests from this address; wait a minute")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Send a form body")
		return
	}
	f := r.PostForm
	if f.Get("grant_type") != "authorization_code" {
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only authorization_code is supported")
		return
	}
	code, verifier, clientID, redirect := f.Get("code"), f.Get("code_verifier"), f.Get("client_id"), f.Get("redirect_uri")
	if code == "" || verifier == "" || clientID == "" || redirect == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code, code_verifier, client_id and redirect_uri are required")
		return
	}
	if res := f.Get("resource"); res != "" && res != s.mcpResource() {
		writeOAuthError(w, http.StatusBadRequest, "invalid_target", "Tokens are only for "+s.mcpResource())
		return
	}
	ctx := r.Context()
	c, err := s.q.UseOAuthCode(ctx, auth.HashToken(code))
	if errors.Is(err, pgx.ErrNoRows) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The code is unknown, used or expired")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sum := sha256.Sum256([]byte(verifier))
	if c.ClientID != clientID || c.RedirectUri != redirect ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(c.CodeChallenge)) != 1 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "The code does not match this client, redirect URI or verifier")
		return
	}
	secret := tokenPrefix + rand.Text() + rand.Text()
	err = s.inTx(ctx, func(q *db.Queries) error {
		client, err := q.GetOAuthClient(ctx, c.ClientID)
		if err != nil {
			return err
		}
		name := client.Name + " (MCP)"
		if rs := []rune(name); len(rs) > 100 {
			name = string(rs[:100])
		}
		t, err := q.CreateAPIToken(ctx, db.CreateAPITokenParams{UserID: c.UserID, Name: name, TokenHash: auth.HashToken(secret), ReadOnly: c.ReadOnly})
		if err != nil {
			return err
		}
		// The user approved this in the browser; audit_events.via has no "oauth".
		m := auditMeta{via: "web", requestID: ptr(requestIDFrom(ctx)), ip: ipAddr(r)}
		return audit(ctx, q, m, &c.UserID, "token", t.ID, "create", map[string]any{"name": name, "read_only": c.ReadOnly, "client_id": client.ID})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": secret, "token_type": "Bearer"})
}
