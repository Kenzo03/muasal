package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// MSL-68: the handover lists the live modules and menus in tree order, each
// with its behaviours in force and open tickets, as far as the reader sees
// them; with a client, only that client's items and the core work.
func TestHandover(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	done := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	e.seedClose(done, statusID(e, w.pm, "Done"), w.pmUser, "HR approves overtime directly.")
	open := e.seedTicket(w.p, w.pmUser, "Overtime export to payroll", nil, w.ot)

	var h httpapi.Handover
	if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/handover", nil, &h); code != http.StatusOK || len(h.Nodes) != 2 {
		t.Fatalf("handover: %d %+v", code, h.Nodes)
	}
	hr, ot := h.Nodes[0], h.Nodes[1]
	if hr.Name != "HR" || hr.Depth != 0 || len(hr.Behaviors)+len(hr.Open) != 0 || ot.Name != "Overtime Approval" || ot.Depth != 1 {
		t.Fatalf("tree: %+v", h.Nodes)
	}
	if len(ot.Behaviors) != 1 || ot.Behaviors[0].Key != done.Key || len(ot.Open) != 1 || ot.Open[0].Key != open.Key || ot.Open[0].Status != "To do" {
		t.Fatalf("Overtime Approval: %+v", ot)
	}

	if code := e.call(admin, http.MethodGet, fmt.Sprintf("/projects/HRIS/handover?client_id=%d", w.b.ID), nil, &h); code != http.StatusOK || len(h.Nodes) != 3 {
		t.Fatalf("admin, Client B: %d %+v", code, h.Nodes)
	}
	for _, n := range h.Nodes {
		if n.Name == "Overtime Approval" && (len(n.Behaviors) != 0 || len(n.Open) != 1) {
			t.Fatalf("Client B sees Client A's behaviour or loses core work: %+v", n)
		}
	}
}
