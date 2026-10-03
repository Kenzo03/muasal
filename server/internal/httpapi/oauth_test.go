package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
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
