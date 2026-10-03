package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// rawJSON posts JSON to a path outside /api/v1 and decodes the reply.
func (e *env) rawJSON(method, path string, body, out any) int {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

// MCP spec: discovery documents point clients at this server's endpoints.
func TestOAuthDiscovery(t *testing.T) {
	e := newEnv(t)
	var pr map[string]any
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
		if code := e.rawJSON(http.MethodGet, path, nil, &pr); code != http.StatusOK || pr["resource"] != origin+"/mcp" {
			t.Fatalf("%s: %d %v", path, code, pr)
		}
	}
	var as map[string]any
	if code := e.rawJSON(http.MethodGet, "/.well-known/oauth-authorization-server", nil, &as); code != http.StatusOK ||
		as["issuer"] != origin || as["authorization_endpoint"] != origin+"/oauth/authorize" ||
		as["token_endpoint"] != origin+"/oauth/token" || as["registration_endpoint"] != origin+"/oauth/register" {
		t.Fatalf("metadata: %d %v", code, as)
	}
}

// MCP spec: dynamic registration takes public clients with safe redirect URIs.
func TestOAuthRegistration(t *testing.T) {
	e := newEnv(t)
	var c map[string]any
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{
		"client_name": "Claude Code", "redirect_uris": []string{"http://localhost:53682/callback", "https://agent.example.com/cb"},
		"grant_types": []string{"authorization_code"}, // extra fields are ignored
	}, &c); code != http.StatusCreated || c["client_id"] == "" || c["client_name"] != "Claude Code" || c["token_endpoint_auth_method"] != "none" {
		t.Fatalf("register: %d %v", code, c)
	}
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"redirect_uris": []string{"http://127.0.0.1:9/cb"}}, &c); code != http.StatusCreated || c["client_name"] != "MCP client" {
		t.Fatalf("unnamed: %d %v", code, c)
	}
	for _, uris := range [][]string{
		{},
		{"http://evil.example.com/cb"},
		{"https://agent.example.com/cb#frag"},
		{"ftp://localhost/cb"},
		{"https://a.example/1", "https://a.example/2", "https://a.example/3", "https://a.example/4", "https://a.example/5", "https://a.example/6"},
	} {
		var bad map[string]any
		if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"client_name": "X", "redirect_uris": uris}, &bad); code != http.StatusBadRequest || bad["error"] != "invalid_redirect_uri" {
			t.Errorf("%v: %d %v", uris, code, bad)
		}
	}
}

// A 43-character PKCE verifier and its S256 challenge (RFC 7636 appendix B).
const (
	verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

func registerTestClient(e *env, redirect string) string {
	e.t.Helper()
	var c map[string]any
	if code := e.rawJSON(http.MethodPost, "/oauth/register", map[string]any{"client_name": "Test Agent", "redirect_uris": []string{redirect}}, &c); code != http.StatusCreated {
		e.t.Fatalf("register: %d %v", code, c)
	}
	return c["client_id"].(string)
}

// approve posts the approval page's request as a signed-in browser and returns the redirect URL.
func approve(e *env, c *http.Client, body map[string]any) (int, string) {
	e.t.Helper()
	var out httpapi.OAuthRedirect
	code := e.call(c, http.MethodPost, "/oauth/approve", body, &out)
	return code, out.RedirectUrl
}

func approveBody(clientID, redirect string, allow bool) map[string]any {
	return map[string]any{"client_id": clientID, "redirect_uri": redirect, "code_challenge": challenge,
		"code_challenge_method": "S256", "state": "st-1", "resource": origin + "/mcp", "read_only": false, "allow": allow}
}

// MCP spec: only a signed-in session approves, only to a registered URI.
func TestOAuthApprove(t *testing.T) {
	e := newEnv(t)
	redirect := "http://localhost:53682/callback"
	id := registerTestClient(e, redirect)
	pm, _ := e.signedIn("pm@example.com", false)

	var got httpapi.OAuthClient
	if code := e.call(pm, http.MethodGet, "/oauth/clients/"+id, nil, &got); code != http.StatusOK || got.Name != "Test Agent" {
		t.Fatalf("client: %d %+v", code, got)
	}
	if code := e.call(pm, http.MethodGet, "/oauth/clients/NOPE", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown client: %d", code)
	}

	code, to := approve(e, pm, approveBody(id, redirect, true))
	u, _ := url.Parse(to)
	if code != http.StatusOK || !strings.HasPrefix(to, redirect+"?") || u.Query().Get("code") == "" || u.Query().Get("state") != "st-1" {
		t.Fatalf("allow: %d %q", code, to)
	}
	if code, to := approve(e, pm, approveBody(id, redirect, false)); code != http.StatusOK || to != redirect+"?error=access_denied&state=st-1" {
		t.Fatalf("deny: %d %q", code, to)
	}
	for name, change := range map[string]func(map[string]any){
		"other redirect":  func(b map[string]any) { b["redirect_uri"] = "http://localhost:1/evil" },
		"plain method":    func(b map[string]any) { b["code_challenge_method"] = "plain" },
		"short challenge": func(b map[string]any) { b["code_challenge"] = "abc" },
		"other resource":  func(b map[string]any) { b["resource"] = "https://elsewhere.example/mcp" },
		"unknown client":  func(b map[string]any) { b["client_id"] = "NOPE" },
	} {
		b := approveBody(id, redirect, true)
		change(b)
		if code, to := approve(e, pm, b); code != http.StatusUnprocessableEntity || to != "" {
			t.Errorf("%s: %d %q", name, code, to)
		}
	}

	// A token can't approve more tokens.
	var tok httpapi.APITokenCreated
	e.call(pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Script", "read_only": false}, &tok)
	var p httpapi.Problem
	if code := e.bearer(tok.Token, http.MethodPost, "/oauth/approve", approveBody(id, redirect, true), &p); code != http.StatusForbidden || p.Code != "session_required" {
		t.Fatalf("approve with a token: %d %+v", code, p)
	}
}
