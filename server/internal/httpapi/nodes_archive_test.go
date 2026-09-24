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

// R-MR-4, AC-MR-5: a linked node is archived, never deleted; it leaves the tree
// and pickers, and its tickets keep the link.
func TestLinkedNodesAreArchivedNotDeleted(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, au, "Overtime rules", &w.a, w.ot)
	path := fmt.Sprintf("/nodes/%d", w.ot.ID)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodDelete, path, nil, &p); code != http.StatusConflict || p.Code != "node_linked" {
		t.Fatalf("delete a linked node: %d %+v", code, p)
	}
	var n httpapi.Node
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": true}, &n); code != http.StatusOK || !n.Archived {
		t.Fatalf("archive: %d %+v", code, n)
	}
	var list httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &list)
	if slices.Contains(nodeNames(list), "Overtime Approval") {
		t.Fatalf("an archived node in the tree: %v", nodeNames(list))
	}
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes?archived=true", nil, &list)
	if i := slices.Index(nodeNames(list), "Overtime Approval"); i < 0 || !list.Items[i].Archived {
		t.Fatalf("an admin lists archived nodes: %+v", list.Items)
	}
	var detail httpapi.Ticket
	if e.call(admin, http.MethodGet, "/tickets/"+tk.Key, nil, &detail); len(detail.Nodes) != 1 || !detail.Nodes[0].Archived {
		t.Fatalf("the ticket keeps the link: %+v", detail.Nodes)
	}
	if code := e.call(admin, http.MethodPatch, path, map[string]any{"archived": false}, &n); code != http.StatusOK || n.Archived {
		t.Fatalf("restore: %d %+v", code, n)
	}
}

func TestArchiveRules(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	node := func(n db.Node) string { return fmt.Sprintf("/nodes/%d", n.ID) }
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPatch, node(w.hr), map[string]any{"archived": true}, &p); code != http.StatusConflict || p.Code != "node_has_children" {
		t.Fatalf("archive a node with live children: %d %+v", code, p)
	}
	for _, n := range []db.Node{w.ot, w.secret, w.hr} {
		if code := e.call(admin, http.MethodPatch, node(n), map[string]any{"archived": true}, nil); code != http.StatusOK {
			t.Fatalf("archive %s: %d", n.Name, code)
		}
	}
	if code := e.call(admin, http.MethodPatch, node(w.ot), map[string]any{"archived": false}, &p); code != http.StatusConflict || p.Code != "parent_archived" {
		t.Fatalf("restore under an archived parent: %d %+v", code, p)
	}
	var list httpapi.NodeList
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/nodes?archived=true", nil, &list); len(list.Items) != 0 {
		t.Fatalf("members never list archived nodes: %+v", list.Items)
	}
	body := map[string]any{"title": "Uses an archived menu", "type": "bug", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "node_ids" {
		t.Fatalf("a new ticket on an archived menu: %d %+v", code, p)
	}
}

func TestClientsUsedByTicketsStayLinked(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	c := e.seedClient("Client C")
	if err := e.q.LinkClients(context.Background(), db.LinkClientsParams{ProjectID: w.p.ID, ClientIds: []int64{c.ID}}); err != nil {
		t.Fatal(err)
	}
	e.seedTicket(w.p, au, "Client C export", &c)
	var p httpapi.Problem
	body := map[string]any{"client_ids": []int64{w.a.ID, w.b.ID}}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/clients", body, &p); code != http.StatusConflict || p.Code != "client_in_use" {
		t.Fatalf("unlink a client with tickets: %d %+v", code, p)
	}
}

func TestInternalContactsFilter(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedContact("Dewi", nil)
	var list httpapi.ContactList
	e.call(w.pm, http.MethodGet, "/contacts?internal=true", nil, &list)
	if len(list.Items) != 1 || list.Items[0].Name != "Dewi" {
		t.Fatalf("internal contacts: %+v", list.Items)
	}
}
