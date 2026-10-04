package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// bearer sends a request with a token and no cookie or Origin, as a script would.
func (e *env) bearer(token, method, path string, body, out any) int {
	e.t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, e.url+"/api/v1"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
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

// §14.3 and R-AC-9: a token acts as its owner, with their visibility; the
// secret shows once; a revoked token gets 401.
func TestTokensActAsTheirOwner(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedTicket(w.p, w.pmUser, "Client A request", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Client B request", &w.b)

	var tok httpapi.APITokenCreated
	if code := e.call(w.pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Sync script", "read_only": false}, &tok); code != http.StatusCreated ||
		!strings.HasPrefix(tok.Token, "msl_") || len(tok.Token) < 40 || tok.ReadOnly {
		t.Fatalf("create: %d %+v", code, tok)
	}
	var list httpapi.APITokenList
	e.call(w.pm, http.MethodGet, "/me/tokens", nil, &list)
	raw, _ := json.Marshal(list)
	if len(list.Items) != 1 || strings.Contains(string(raw), tok.Token) {
		t.Fatalf("list: %s", raw)
	}

	var page httpapi.TicketPage
	if code := e.bearer(tok.Token, http.MethodGet, "/projects/HRIS/tickets", nil, &page); code != http.StatusOK || len(page.Items) != 1 || page.Items[0].Title != "Client A request" {
		t.Fatalf("tickets as the member: %d %+v", code, page.Items)
	}
	// Writes need no Origin with a token, and the history says they came through the API.
	var created httpapi.Ticket
	if code := e.bearer(tok.Token, http.MethodPost, "/projects/HRIS/tickets", map[string]any{"type": "bug", "title": "Filed by a script", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}, &created); code != http.StatusCreated {
		t.Fatalf("create through the API: %d", code)
	}
	events, _ := e.q.ListTicketEvents(t.Context(), created.Id)
	var via string
	e.d.Pool.QueryRow(t.Context(), "SELECT via FROM audit_events WHERE id = $1", events[0].ID).Scan(&via)
	if via != "api" {
		t.Fatalf("audit via %q", via)
	}
	// MSL-27: the audit log names the token and the ticket by its key.
	admin, _ := e.signedIn("admin@example.com", true)
	var audit httpapi.AuditPage
	if code := e.call(admin, http.MethodGet, "/admin/audit?entity=ticket&action=create", nil, &audit); code != http.StatusOK || len(audit.Items) == 0 ||
		audit.Items[0].Token == nil || *audit.Items[0].Token != "Sync script" || audit.Items[0].Subject == nil || *audit.Items[0].Subject != created.Key {
		t.Fatalf("audit: %d %+v", code, audit.Items)
	}

	var p httpapi.Problem
	if code := e.bearer(tok.Token, http.MethodPost, "/me/tokens", map[string]any{"name": "Another", "read_only": true}, &p); code != http.StatusForbidden || p.Code != "session_required" {
		t.Fatalf("a token minting tokens: %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/me/tokens/%d", tok.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("revoke: %d", code)
	}
	if code := e.bearer(tok.Token, http.MethodGet, "/me", nil, &p); code != http.StatusUnauthorized || p.Code != "invalid_token" {
		t.Fatalf("a revoked token: %d %+v", code, p)
	}
	if code := e.bearer("msl_not-a-real-token", http.MethodGet, "/me", nil, &p); code != http.StatusUnauthorized {
		t.Fatalf("an unknown token: %d", code)
	}
}

// AC-IN-4: a read-only token cannot create a ticket, but reads and asks.
func TestReadOnlyTokensOnlyRead(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var tok httpapi.APITokenCreated
	e.call(w.pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Reporting", "read_only": true}, &tok)
	var p httpapi.Problem
	if code := e.bearer(tok.Token, http.MethodPost, "/projects/HRIS/tickets", map[string]any{"type": "bug", "title": "Should not be filed", "node_ids": []int64{w.ot.ID}}, &p); code != http.StatusForbidden || p.Code != "token_read_only" {
		t.Fatalf("read-only create: %d %+v", code, p)
	}
	if code := e.bearer(tok.Token, http.MethodGet, "/projects/HRIS/tickets", nil, nil); code != http.StatusOK {
		t.Fatalf("read-only read: %d", code)
	}
	if code := e.bearer(tok.Token, http.MethodPost, "/ask", map[string]any{"question": "Why does overtime skip the supervisor?"}, nil); code != http.StatusOK {
		t.Fatalf("read-only ask: %d", code)
	}
}

// §14.3: an expired token gets 401, and a past expiry is refused.
func TestExpiredTokensAreRefused(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Old", "read_only": true, "expires_on": "2020-01-01"}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("past expiry: %d", code)
	}
	var tok httpapi.APITokenCreated
	e.call(w.pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Soon", "read_only": true, "expires_on": "2099-01-01"}, &tok)
	if tok.ExpiresAt == nil {
		t.Fatalf("expiry: %+v", tok)
	}
	if _, err := e.d.Pool.Exec(t.Context(), "UPDATE api_tokens SET expires_at = now() - interval '1 second' WHERE id = $1", tok.Id); err != nil {
		t.Fatal(err)
	}
	if code := e.bearer(tok.Token, http.MethodGet, "/me", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("expired: %d", code)
	}
}

// §14.3: a token takes 60 requests a minute, bursting to 120; then 429.
func TestTokensAreRateLimited(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var tok httpapi.APITokenCreated
	e.call(w.pm, http.MethodPost, "/me/tokens", map[string]any{"name": "Busy", "read_only": true}, &tok)
	for i := range 120 {
		if code := e.bearer(tok.Token, http.MethodGet, "/me", nil, nil); code != http.StatusOK {
			t.Fatalf("request %d: %d", i+1, code)
		}
	}
	var p httpapi.Problem
	if code := e.bearer(tok.Token, http.MethodGet, "/me", nil, &p); code != http.StatusTooManyRequests || p.Code != "rate_limited" {
		t.Fatalf("request 121: %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodGet, "/me", nil, nil); code != http.StatusOK {
		t.Fatalf("the owner's session is not limited: %d", code)
	}
}

// §17.1: a retried create with the same Idempotency-Key returns the first
// ticket instead of a second one; another key creates another ticket.
func TestIdempotentTicketCreation(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	body := map[string]any{"type": "bug", "title": "Created by a flaky network", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}
	var first, again, other httpapi.Ticket
	code, _ := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]string{"Idempotency-Key": "sync-42"}, body, &first)
	if code != http.StatusCreated {
		t.Fatalf("first: %d", code)
	}
	code, h := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]string{"Idempotency-Key": "sync-42"}, body, &again)
	if code != http.StatusOK || h.Get("Idempotent-Replayed") != "true" || again.Key != first.Key {
		t.Fatalf("retry: %d %q %s", code, h.Get("Idempotent-Replayed"), again.Key)
	}
	e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]string{"Idempotency-Key": "sync-43"}, body, &other)
	if other.Key == first.Key {
		t.Fatalf("another key reused %s", other.Key)
	}
	var n int
	e.d.Pool.QueryRow(t.Context(), "SELECT count(*) FROM tickets").Scan(&n)
	if n != 2 {
		t.Fatalf("%d tickets", n)
	}
}

// A replayed key answers with its ticket only in the project it was created
// in and only while the caller still sees it; otherwise it is a conflict.
func TestIdempotentReplayNeedsTheTicketsProject(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ops := e.seedProject("OPS")
	opsNode := e.seedNode(ops, nil, "menu", "Shipping")
	e.seedMember(w.pmUser, ops, "member")
	key := map[string]string{"Idempotency-Key": "sync-42"}
	var first httpapi.Ticket
	if code, _ := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", key,
		map[string]any{"type": "bug", "title": "Created by a flaky network", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}, &first); code != http.StatusCreated {
		t.Fatalf("first: %d", code)
	}
	e.exec("DELETE FROM memberships WHERE user_id = $1 AND project_id = $2", w.pmUser.ID, w.p.ID)
	var p httpapi.Problem
	code, _ := e.callWith(w.pm, http.MethodPost, "/projects/OPS/tickets", key,
		map[string]any{"type": "bug", "title": "Created by a flaky network", "node_ids": []int64{opsNode.ID}}, &p)
	if code != http.StatusConflict || p.Code != "idempotency_conflict" {
		t.Fatalf("replay after removal: %d %+v", code, p)
	}

	// Same project, but the caller no longer sees the ticket's client.
	key = map[string]string{"Idempotency-Key": "sync-44"}
	e.seedMember(w.pmUser, w.p, "member", w.a)
	if code, _ := e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", key,
		map[string]any{"type": "bug", "title": "Created for Client A", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}, &first); code != http.StatusCreated {
		t.Fatalf("second: %d", code)
	}
	e.exec("DELETE FROM membership_clients WHERE user_id = $1 AND project_id = $2", w.pmUser.ID, w.p.ID)
	e.seedMember(w.pmUser, w.p, "member", w.b)
	p = httpapi.Problem{}
	code, _ = e.callWith(w.pm, http.MethodPost, "/projects/HRIS/tickets", key,
		map[string]any{"type": "bug", "title": "Created for Client A", "node_ids": []int64{w.secret.ID}, "client_id": w.b.ID}, &p)
	if code != http.StatusConflict || p.Code != "idempotency_conflict" {
		t.Fatalf("replay out of scope: %d %+v", code, p)
	}
}
