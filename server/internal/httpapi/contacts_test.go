package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestContactsFollowTheClientScope(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	e.seedContact("Dewi", nil)
	e.seedContact("Andi", &a)
	bayu := e.seedContact("Bayu", &b)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b)
	outsider, _ := e.signedIn("out@example.com", false)

	names := func(c *http.Client, query string) []string {
		var list httpapi.ContactList
		if code := e.call(c, http.MethodGet, "/contacts"+query, nil, &list); code != http.StatusOK {
			t.Fatalf("list %s: %d", query, code)
		}
		out := []string{}
		for _, it := range list.Items {
			out = append(out, it.Name)
		}
		return out
	}
	if got := names(budi, ""); !slices.Equal(got, []string{"Bayu", "Dewi"}) {
		t.Fatalf("scoped to B: %v", got)
	}
	if got := names(budi, "?q=ba"); !slices.Equal(got, []string{"Bayu"}) {
		t.Fatalf("search: %v", got)
	}
	if got := names(budi, fmt.Sprintf("?client_id=%d", a.ID)); len(got) != 0 {
		t.Fatalf("an out-of-scope filter finds nothing: %v", got)
	}
	if got := names(outsider, ""); len(got) != 0 {
		t.Fatalf("no project, no contacts: %v", got)
	}

	var c httpapi.Contact
	body := map[string]any{"name": "Budi Santoso", "client_id": b.ID, "title": "HR Manager", "email": "budi@client-b.example"}
	if code := e.call(budi, http.MethodPost, "/contacts", body, &c); code != http.StatusCreated ||
		c.ClientName == nil || *c.ClientName != "Client B" || c.Title == nil || *c.Title != "HR Manager" {
		t.Fatalf("create: %d %+v", code, c)
	}
	var prob httpapi.Problem
	if code := e.call(budi, http.MethodPost, "/contacts", map[string]any{"name": "X", "client_id": a.ID}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "client_id" {
		t.Fatalf("out-of-scope client: %d %+v", code, prob)
	}
	if code := e.call(budi, http.MethodPost, "/contacts", map[string]any{"name": "X", "email": "not-an-email"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "email" {
		t.Fatalf("bad email: %d %+v", code, prob)
	}
	// PATCH replaces every field: without client_id, Bayu becomes internal and stays visible.
	if code := e.call(budi, http.MethodPatch, fmt.Sprintf("/contacts/%d", bayu), map[string]any{"name": "Bayu"}, &c); code != http.StatusOK || c.ClientId != nil {
		t.Fatalf("make internal: %d %+v", code, c)
	}
}

func TestViewersReadContactsButDoNotWriteThem(t *testing.T) {
	e := newEnv(t)
	a := e.seedClient("Client A")
	p := e.seedProject("HRIS", a)
	andi := e.seedContact("Andi", &a)
	viewer, vu := e.signedIn("viewer@example.com", false)
	e.seedMember(vu, p, "viewer")
	if code := e.call(viewer, http.MethodPost, "/contacts", map[string]any{"name": "X"}, nil); code != http.StatusForbidden {
		t.Fatalf("viewer creates: %d", code)
	}
	if code := e.call(viewer, http.MethodPatch, fmt.Sprintf("/contacts/%d", andi), map[string]any{"name": "Andi", "client_id": a.ID}, nil); code != http.StatusForbidden {
		t.Fatalf("viewer edits: %d", code)
	}
	if code := e.call(viewer, http.MethodPatch, "/contacts/999999", map[string]any{"name": "X"}, nil); code != http.StatusNotFound {
		t.Fatalf("missing contact: %d", code)
	}
}
