package httpapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// mcpPost sends one JSON-RPC message to /mcp.
func mcpPost(e *env, token string, msg map[string]any) (*http.Response, []byte) {
	e.t.Helper()
	b, _ := json.Marshal(msg)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, body
}

// mcpCall calls one tool and returns its text and whether it is an error.
func mcpCall(e *env, token, tool string, args map[string]any) (string, bool) {
	e.t.Helper()
	res, body := mcpPost(e, token, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	var out struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
			IsError bool                    `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil || res.StatusCode != http.StatusOK || out.Error != nil || len(out.Result.Content) == 0 {
		e.t.Fatalf("%s: %d %s", tool, res.StatusCode, body)
	}
	return out.Result.Content[0].Text, out.Result.IsError
}

// MCP spec: /mcp asks for sign-in, and takes only a token.
func TestMCPNeedsAToken(t *testing.T) {
	e := newEnv(t)
	hello := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "t", "version": "1"}}}
	res, _ := mcpPost(e, "", hello)
	if res.StatusCode != http.StatusUnauthorized ||
		res.Header.Get("WWW-Authenticate") != `Bearer resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp"` {
		t.Fatalf("no token: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
	if res, _ := mcpPost(e, "msl_nope", hello); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", res.StatusCode)
	}
	// A browser session is not enough: a web page must not drive the tools.
	pm, _ := e.signedIn("pm@example.com", false)
	b, _ := json.Marshal(hello)
	req, _ := http.NewRequest(http.MethodPost, e.url+"/mcp", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if r, err := pm.Do(req); err != nil || r.StatusCode != http.StatusUnauthorized {
		t.Fatalf("session: %v %d", err, r.StatusCode)
	}
}

// The read tools see what the token's user sees, no more.
func TestMCPReadTools(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedTicket(w.p, w.pmUser, "Client A request", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Client B request", &w.b, w.secret)
	tok := mcpToken(e, w.pm, true)

	res, body := mcpPost(e, tok, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	for _, name := range []string{"get_project", "list_tickets", "get_ticket"} {
		if res.StatusCode != http.StatusOK || !strings.Contains(string(body), `"`+name+`"`) {
			t.Fatalf("tools/list lacks %s: %d %s", name, res.StatusCode, body)
		}
	}
	text, isErr := mcpCall(e, tok, "list_tickets", map[string]any{"project": "HRIS"})
	if isErr || !strings.Contains(text, "Client A request") || strings.Contains(text, "Client B request") {
		t.Fatalf("list_tickets: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-1"}); isErr || !strings.Contains(text, `"version":1`) {
		t.Fatalf("get_ticket: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-2"}); !isErr || !strings.Contains(text, "404") {
		t.Fatalf("hidden ticket: %v %s", isErr, text)
	}
	text, isErr = mcpCall(e, tok, "get_project", map[string]any{"project": "HRIS"})
	if isErr || !strings.Contains(text, `"statuses"`) || !strings.Contains(text, "Overtime Approval") || strings.Contains(text, "Client B Report") || !strings.Contains(text, `"cancelled"`) {
		t.Fatalf("get_project: %v %s", isErr, text)
	}
}
