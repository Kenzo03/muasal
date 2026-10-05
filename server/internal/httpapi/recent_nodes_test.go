package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// FSD §8.1: the menu picker puts the caller's recently used menus first: those
// of their own latest tickets, live and visible to them, most recent first.
func TestRecentNodesAreTheCallersOwnLatest(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	other, ou := e.signedIn("ani@example.com", false)
	e.seedMember(ou, w.p, "member")
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	archived := e.seedNode(w.p, &w.hr, "menu", "Old Menu")
	create := func(c *http.Client, title string, nodes ...int64) {
		t.Helper()
		if code := e.call(c, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
			"type": "change_request", "title": title, "node_ids": nodes, "requester_user_id": w.pmUser.ID,
		}, nil); code != http.StatusCreated {
			t.Fatalf("create %q: %d", title, code)
		}
	}
	create(w.pm, "Overtime export", w.ot.ID)
	create(w.pm, "Leave carry-over", leave.ID, archived.ID)
	create(w.pm, "Overtime cap", w.ot.ID) // Overtime Approval is used most recently again
	create(other, "Someone else's ticket", w.hr.ID)
	e.seedTicket(w.p, w.pmUser, "On a menu the PM cannot see", nil, w.secret)
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE nodes SET archived_at = now() WHERE id = $1", archived.ID); err != nil {
		t.Fatal(err)
	}
	var got httpapi.RecentNodes
	if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/nodes/recent", nil, &got); code != http.StatusOK ||
		!slices.Equal(got.NodeIds, []int64{w.ot.ID, leave.ID}) {
		t.Fatalf("recent: %d %v, want [%d %d]", code, got.NodeIds, w.ot.ID, leave.ID)
	}
	if code := e.call(other, http.MethodGet, "/projects/HRIS/nodes/recent", nil, &got); code != http.StatusOK || !slices.Equal(got.NodeIds, []int64{w.hr.ID}) {
		t.Fatalf("another member's recent: %d %v", code, got.NodeIds)
	}
}
