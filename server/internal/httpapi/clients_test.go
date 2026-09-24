package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestSystemAdminsManageClients(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	var c httpapi.Client
	code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": " Client A ", "code": "CLA", "aliases": []string{" PT Alfa ", ""}}, &c)
	if code != http.StatusCreated || c.Name != "Client A" || c.Code == nil || *c.Code != "CLA" ||
		!slices.Equal(c.Aliases, []string{"PT Alfa"}) || c.Archived {
		t.Fatalf("create: %d %+v", code, c)
	}
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/clients", map[string]any{"name": "client a"}, &p); code != http.StatusConflict || p.Code != "client_name_taken" {
		t.Fatalf("duplicate name: %d %+v", code, p)
	}
	path := fmt.Sprintf("/clients/%d", c.Id)
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": true, "code": ""}, &c); code != http.StatusOK ||
		!c.Archived || c.Code != nil || c.Name != "Client A" || len(c.Aliases) != 1 {
		t.Fatalf("archive: %d %+v", code, c)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "client", EntityID: c.Id})
	if err != nil || len(events) != 2 || events[1].Action != "update" {
		t.Fatalf("audit: %+v %v", events, err)
	}
}

func TestProjectAdminsCreateClientsButOnlySystemAdminsEditThem(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member")
	var c httpapi.Client
	if code := e.call(owner, http.MethodPost, "/clients", map[string]any{"name": "Client B"}, &c); code != http.StatusCreated {
		t.Fatalf("project admin creates: %d", code)
	}
	if code := e.call(owner, http.MethodGet, "/clients", nil, nil); code != http.StatusOK {
		t.Fatalf("project admin lists: %d", code)
	}
	if code := e.call(owner, http.MethodPatch, fmt.Sprintf("/clients/%d", c.Id), map[string]any{"name": "X"}, nil); code != http.StatusForbidden {
		t.Fatalf("project admin renames: %d", code)
	}
	if code := e.call(member, http.MethodGet, "/clients", nil, nil); code != http.StatusForbidden {
		t.Fatalf("member lists: %d", code)
	}
	if code := e.call(member, http.MethodPost, "/clients", map[string]any{"name": "Client C"}, nil); code != http.StatusForbidden {
		t.Fatalf("member creates: %d", code)
	}
}

func TestLinkingClientsToAProject(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.ClientList
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID, b.ID}}, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("link: %d %+v", code, list)
	}
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b) // a member scope now uses Client B
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID}}, &prob); code != http.StatusConflict || prob.Code != "client_in_use" {
		t.Fatalf("unlink a client in use: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", map[string]any{"client_ids": []int64{a.ID, b.ID, 999999}}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "unknown_client" {
		t.Fatalf("unknown client: %d %+v", code, prob)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS/clients", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member reads the links: %d", code)
	}
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/clients", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("failed updates must change nothing: %d %+v", code, list)
	}
}
