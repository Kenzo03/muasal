package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// AC-TK-8, R-TK-5 and R-TK-7: HRIS-2 reverses HRIS-1, so HRIS-1's decision is
// superseded on its ticket page and on the node timeline, and leaves the
// behaviors in force; removing the link makes it current again.
func TestReversingSupersedesTheDecision(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	done := statusID(e, w.pm, "Done")
	old := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	e.seedClose(old, done, w.pmUser, "Overtime approval skips the supervisor for Client A.")
	now := e.seedTicket(w.p, w.pmUser, "Bring back supervisor approval", &w.a, w.ot)

	var link httpapi.TicketLink
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+now.Key+"/links", map[string]any{"type": "reverses", "key": "hris-1"}, &link); code != http.StatusCreated ||
		!link.Outgoing || link.Type != httpapi.LinkTypeReverses || link.Ticket.Key != old.Key {
		t.Fatalf("create: %d %+v", code, link)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+old.Key, nil, &got)
	if got.Decision == nil || got.Decision.SupersededBy == nil || *got.Decision.SupersededBy != now.Key ||
		len(got.Links) != 1 || got.Links[0].Outgoing || got.Links[0].Ticket.Key != now.Key {
		t.Fatalf("reversed ticket: %+v %+v", got.Decision, got.Links)
	}
	var tl httpapi.TimelinePage
	e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline", w.ot.ID), nil, &tl)
	i := slices.IndexFunc(tl.Items, func(it httpapi.TimelineEntry) bool { return it.Key == old.Key })
	if i < 0 || tl.Items[i].Decision == nil || tl.Items[i].Decision.SupersededBy == nil || *tl.Items[i].Decision.SupersededBy != now.Key {
		t.Fatalf("timeline: %+v", tl.Items)
	}
	var b httpapi.BehaviorList
	if e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/behaviors", w.ot.ID), nil, &b); len(b.Items) != 0 {
		t.Fatalf("a superseded decision is still in force: %+v", b.Items)
	}
	if acts := ticketActions(e, old.ID); acts[len(acts)-1] != "link" {
		t.Fatalf("history: %v", acts)
	}

	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/links/%d", link.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	var after httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+old.Key, nil, &after)
	if after.Decision.SupersededBy != nil || len(after.Links) != 0 {
		t.Fatalf("after unlink: %+v %+v", after.Decision, after.Links)
	}
	if e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/behaviors", w.ot.ID), nil, &b); len(b.Items) != 1 {
		t.Fatalf("the decision is not current again: %+v", b.Items)
	}
}

// R-TK-5: a ticket closed after the link that reverses it is superseded at once.
func TestADecisionConfirmedAfterItsReversalIsSuperseded(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	old := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	now := e.seedTicket(w.p, w.pmUser, "Bring back supervisor approval", &w.a, w.ot)
	e.call(w.pm, http.MethodPost, "/tickets/"+now.Key+"/links", map[string]any{"type": "reverses", "key": old.Key}, nil)
	body := map[string]any{
		"status_id": statusID(e, w.pm, "Done"), "reason": "Leave periods.", "node_ids": []int64{w.ot.ID},
		"decision": decisionBody("Skip the supervisor for Client A.", "Approvals stalled."),
	}
	var out httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+old.Key+"/transition", body, &out); code != http.StatusOK ||
		out.Decision == nil || out.Decision.SupersededBy == nil || *out.Decision.SupersededBy != now.Key {
		t.Fatalf("close: %d %+v", code, out.Decision)
	}
}

// R-TK-6: no self-links, no duplicates, and no links to tickets the member
// cannot see; links to hidden tickets stay hidden.
func TestLinksNeedTwoVisibleDistinctTickets(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	a := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	b := e.seedTicket(w.p, w.pmUser, "Overtime cap", nil, w.ot)
	hidden := e.seedTicket(w.p, w.pmUser, "Client B report", &w.b, w.secret)
	path := "/tickets/" + a.Key + "/links"

	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "related_to", "key": a.Key}, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "self_link" {
		t.Fatalf("self-link: %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "related_to", "key": hidden.Key}, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "not_found" {
		t.Fatalf("hidden target: %d %+v", code, p)
	}
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "extends", "key": b.Key}, nil); code != http.StatusCreated {
		t.Fatalf("extends: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "extends", "key": b.Key}, &p); code != http.StatusConflict || p.Code != "link_exists" {
		t.Fatalf("duplicate: %d %+v", code, p)
	}

	// An admin links A to the Client B ticket; the member scoped to Client A never sees that link.
	admin, _ := e.signedIn("admin@example.com", true)
	if code := e.call(admin, http.MethodPost, path, map[string]any{"type": "related_to", "key": hidden.Key}, nil); code != http.StatusCreated {
		t.Fatalf("admin link: %d", code)
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+a.Key, nil, &got)
	if len(got.Links) != 1 || got.Links[0].Ticket.Key != b.Key {
		t.Fatalf("member sees: %+v", got.Links)
	}
	var all httpapi.Ticket
	e.call(admin, http.MethodGet, "/tickets/"+a.Key, nil, &all)
	if len(all.Links) != 2 {
		t.Fatalf("admin sees: %+v", all.Links)
	}
}

// A reverses link supersedes the other ticket's decision, so it needs the
// Member role in that ticket's project too; seeing the ticket is enough for
// other links.
func TestReversingNeedsMemberOnTheOtherProject(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ops := e.seedProject("OPS")
	e.seedMember(w.pmUser, ops, "viewer")
	theirs := e.seedTicket(ops, w.pmUser, "Ship from the north warehouse", nil)
	mine := e.seedTicket(w.p, w.pmUser, "Bring back supervisor approval", &w.a, w.ot)
	path := "/tickets/" + mine.Key + "/links"
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "reverses", "key": theirs.Key}, nil); code != http.StatusForbidden {
		t.Fatalf("reverses as a viewer of the other project: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, path, map[string]any{"type": "related_to", "key": theirs.Key}, nil); code != http.StatusCreated {
		t.Fatalf("related to: %d", code)
	}

	// Removing a reversal makes the other decision current again, so it needs
	// the same role.
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "member")
	e.seedMember(leadUser, ops, "member")
	var done int64
	if err := e.d.Pool.QueryRow(t.Context(), "SELECT id FROM statuses WHERE project_id = $1 AND name = 'Done'", ops.ID).Scan(&done); err != nil {
		t.Fatal(err)
	}
	e.seedClose(theirs, done, leadUser, "Orders ship from the north warehouse.")
	var link httpapi.TicketLink
	if code := e.call(lead, http.MethodPost, path, map[string]any{"type": "reverses", "key": theirs.Key}, &link); code != http.StatusCreated {
		t.Fatalf("reverses as a member of both: %d", code)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/links/%d", link.Id), nil, nil); code != http.StatusForbidden {
		t.Fatalf("unlink as a viewer of the other project: %d", code)
	}
	var got httpapi.Ticket
	e.call(lead, http.MethodGet, "/tickets/"+theirs.Key, nil, &got)
	if got.Decision == nil || got.Decision.SupersededBy == nil || *got.Decision.SupersededBy != mine.Key {
		t.Fatalf("the reversal was undone: %+v", got.Decision)
	}
}
