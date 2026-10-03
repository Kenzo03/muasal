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
	if res, _ := mcpPost(e, "msl_nope", hello); res.StatusCode != http.StatusUnauthorized ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), `error="invalid_token"`) ||
		!strings.Contains(res.Header.Get("WWW-Authenticate"), `resource_metadata="`+origin+`/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("bad token: %d %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
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

// The write tools go through the API's rules: read-only tokens, versions, close validation.
func TestMCPWriteTools(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tok := mcpToken(e, w.pm, false)

	text, isErr := mcpCall(e, tok, "create_ticket", map[string]any{"project": "HRIS", "type": "bug", "title": "Filed by an agent",
		"node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "reason": "The agent found it."})
	var tk struct {
		Key     string
		Title   string
		Reason  string
		Version int
		Status  struct{ Category string }
	}
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Key != "HRIS-1" {
		t.Fatalf("create: %v %s", isErr, text)
	}

	// Only the given field changes.
	text, isErr = mcpCall(e, tok, "update_ticket", map[string]any{"key": "HRIS-1", "title": "Renamed by an agent"})
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Title != "Renamed by an agent" || tk.Reason != "The agent found it." {
		t.Fatalf("update: %v %s", isErr, text)
	}
	if text, isErr := mcpCall(e, tok, "update_ticket", map[string]any{"key": "HRIS-1", "title": "Stale", "version": 1}); !isErr || !strings.Contains(text, "412") {
		t.Fatalf("stale version: %v %s", isErr, text)
	}

	// Cancel needs why; with it the ticket closes as Cancelled with a decision record.
	if text, isErr := mcpCall(e, tok, "cancel_ticket", map[string]any{"key": "HRIS-1", "reason": "Duplicate of another ticket.", "why": ""}); !isErr || !strings.Contains(text, "422") {
		t.Fatalf("cancel without why: %v %s", isErr, text)
	}
	text, isErr = mcpCall(e, tok, "cancel_ticket", map[string]any{"key": "HRIS-1", "reason": "Duplicate of another ticket.", "why": "The same request was filed twice."})
	if isErr || json.Unmarshal([]byte(text), &tk) != nil || tk.Status.Category != "cancelled" {
		t.Fatalf("cancel: %v %s", isErr, text)
	}
	if text, _ := mcpCall(e, tok, "get_ticket", map[string]any{"key": "HRIS-1"}); !strings.Contains(text, "The same request was filed twice.") {
		t.Fatalf("decision record: %s", text)
	}

	// transition_ticket reopens it.
	var statuses struct {
		Items []struct {
			Id       int64
			Category string
		}
	}
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &statuses)
	var todo int64
	for _, s := range statuses.Items {
		if s.Category == "todo" {
			todo = s.Id
		}
	}
	if text, isErr := mcpCall(e, tok, "transition_ticket", map[string]any{"key": "HRIS-1", "status_id": todo}); isErr || !strings.Contains(text, `"category":"todo"`) {
		t.Fatalf("reopen: %v %s", isErr, text)
	}

	// A read-only token can't write.
	ro := mcpToken(e, w.pm, true)
	if text, isErr := mcpCall(e, ro, "create_ticket", map[string]any{"project": "HRIS", "type": "bug", "title": "No", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}); !isErr || !strings.Contains(text, "token_read_only") {
		t.Fatalf("read-only create: %v %s", isErr, text)
	}
}
