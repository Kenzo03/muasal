package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// R-MR-6: merging a duplicate moves its tickets, notes and sub-nodes to the
// target, keeps its name as an alias, and archives it.
func TestMergeFoldsADuplicateNode(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	dup := e.seedNode(w.p, &w.hr, "menu", "OT Approval")
	child := e.seedNode(w.p, &dup, "menu", "OT Rules")
	tk := e.seedTicket(w.p, w.pmUser, "Overtime under the duplicate", &w.a, dup)
	both := e.seedTicket(w.p, w.pmUser, "Overtime on both", &w.a, dup, w.ot)
	e.seedNote(w.p, w.pmUser, "Meeting on the duplicate", nil, dup)

	path := fmt.Sprintf("/nodes/%d/merge", dup.ID)
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"into_id": w.ot.ID}, &p); code != http.StatusForbidden {
		t.Fatalf("a member merging: %d", code)
	}
	if code := e.call(admin, http.MethodPost, path, map[string]any{"into_id": child.ID}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("merging into a child: %d %+v", code, p)
	}
	var out httpapi.Node
	if code := e.call(admin, http.MethodPost, path, map[string]any{"into_id": w.ot.ID}, &out); code != http.StatusOK || !slices.Contains(out.Aliases, "OT Approval") {
		t.Fatalf("merge: %d %+v", code, out)
	}
	for _, key := range []string{tk.Key, both.Key} {
		var got httpapi.Ticket
		e.call(admin, http.MethodGet, "/tickets/"+key, nil, &got)
		if len(got.Nodes) != 1 || got.Nodes[0].Id != w.ot.ID {
			t.Fatalf("%s nodes: %+v", key, got.Nodes)
		}
	}
	var tl httpapi.TimelinePage
	e.call(admin, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline?sub_nodes=false", w.ot.ID), nil, &tl)
	if len(tl.Items) != 2 || len(tl.Notes) != 1 {
		t.Fatalf("target timeline: %d tickets, %d notes", len(tl.Items), len(tl.Notes))
	}
	var gone, moved httpapi.NodeDetail
	e.call(admin, http.MethodGet, fmt.Sprintf("/nodes/%d", dup.ID), nil, &gone)
	e.call(admin, http.MethodGet, fmt.Sprintf("/nodes/%d", child.ID), nil, &moved)
	if !gone.Node.Archived || moved.Node.ParentId == nil || *moved.Node.ParentId != w.ot.ID {
		t.Fatalf("duplicate %+v, child %+v", gone.Node, moved.Node)
	}
}
