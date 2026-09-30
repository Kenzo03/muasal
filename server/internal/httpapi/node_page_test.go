package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// timelineKeys lists a timeline's ticket keys in the order the API gave them.
func timelineKeys(e *env, c *http.Client, path string) []string {
	e.t.Helper()
	var page httpapi.TimelinePage
	if code := e.call(c, http.MethodGet, path, nil, &page); code != http.StatusOK {
		e.t.Fatalf("GET %s: %d", path, code)
	}
	keys := []string{}
	for _, it := range page.Items {
		keys = append(keys, it.Key)
	}
	return keys
}

// AC-MR-5: an archived node's page still opens, with its path; a node hidden
// by the client scope looks missing (R-AC-5).
func TestNodePageReadsArchivedNodesWithTheirPath(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	if code := e.call(admin, http.MethodPatch, fmt.Sprintf("/nodes/%d", w.ot.ID), map[string]any{"archived": true}, nil); code != http.StatusOK {
		t.Fatalf("archive: %d", code)
	}
	var got httpapi.NodeDetail
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d", w.ot.ID), nil, &got); code != http.StatusOK || !got.Node.Archived ||
		got.Node.Name != "Overtime Approval" || got.ProjectKey != "HRIS" || !slices.Equal(got.Path, []httpapi.Ref{{Id: w.hr.ID, Name: "HR"}}) {
		t.Fatalf("node page: %d %+v", code, got)
	}
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d", w.secret.ID), nil, nil); code != http.StatusNotFound {
		t.Fatalf("a hidden node: %d", code)
	}
}

// AC-MR-3: a menu's timeline lists its closed tickets newest first by close
// date, each with what changed and why; open tickets come first (FSD §7.4).
func TestNodeTimelineListsHistoryNewestFirst(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	done := statusID(e, w.pm, "Done")
	for i, closedOn := range []string{"2024-03-01", "2026-02-02", "2025-06-10"} {
		tk := e.seedTicket(w.p, w.pmUser, fmt.Sprintf("Overtime change %d", i+1), &w.a, w.ot)
		e.seedClose(tk, done, w.pmUser, fmt.Sprintf("Change %d", i+1))
		at, err := time.Parse(time.DateOnly, closedOn)
		if err == nil {
			_, err = e.d.Pool.Exec(ctx, "UPDATE tickets SET closed_at = $2 WHERE id = $1", tk.ID, at)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	open := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	var page httpapi.TimelinePage
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline", w.ot.ID), nil, &page); code != http.StatusOK || len(page.Items) != 4 {
		t.Fatalf("timeline: %d %+v", code, page)
	}
	var keys []string
	for _, it := range page.Items {
		keys = append(keys, it.Key)
	}
	if want := []string{open.Key, "HRIS-2", "HRIS-3", "HRIS-1"}; !slices.Equal(keys, want) {
		t.Fatalf("order: %v, want %v", keys, want)
	}
	first, latest := page.Items[0], page.Items[1]
	if first.ClosedAt != nil || first.Decision != nil || first.Client == nil || first.Client.Name != "Client A" || first.Requester.Name != "pm@example.com" {
		t.Fatalf("the open entry: %+v", first)
	}
	if latest.ClosedAt == nil || latest.Decision == nil || latest.Decision.WhatChanged != "Change 2" || latest.Decision.Why == "" || latest.Status.Name != "Done" {
		t.Fatalf("the latest close: %+v", latest)
	}
}

// AC-MR-4: a module's timeline holds its menus' tickets unless sub-nodes are
// left out; the filters narrow it, and hidden tickets never show (R-AC-7).
func TestNodeTimelineIncludesSubNodesByDefault(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	onModule := e.seedTicket(w.p, w.pmUser, "HR module rename", nil, w.hr)
	onOT := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a, w.ot)
	onLeave := e.seedTicket(w.p, w.pmUser, "Leave carry-over", nil, leave)
	e.seedTicket(w.p, w.pmUser, "Client B report", &w.b, w.secret) // hidden from the PM, who sees Client A
	hr := fmt.Sprintf("/nodes/%d/timeline", w.hr.ID)
	sorted := func(keys []string) []string {
		slices.Sort(keys)
		return keys
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{"", []string{onModule.Key, onOT.Key, onLeave.Key}},
		{"?sub_nodes=false", []string{onModule.Key}},
		{"?core=true", []string{onModule.Key, onLeave.Key}},
		{fmt.Sprintf("?client_id=%d", w.a.ID), []string{onOT.Key}},
		{"?type=bug", []string{}},
		{"?from=2999-01-01", []string{}},
	} {
		if got := sorted(timelineKeys(e, w.pm, hr+c.query)); !slices.Equal(got, sorted(c.want)) {
			t.Errorf("timeline%s: %v, want %v", c.query, got, c.want)
		}
	}
}

// Story 5: the Behaviors tab lists the decisions in force, core work first,
// then by client; a Cancelled decision and an open ticket are no behavior.
func TestNodeBehaviorsListDecisionsInForceByClient(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	done, cancelled := statusID(e, admin, "Done"), statusID(e, admin, "Cancelled")
	core := e.seedTicket(w.p, w.pmUser, "Core rule", nil, w.hr)
	forA := e.seedTicket(w.p, w.pmUser, "Client A rule", &w.a, w.ot)
	forB := e.seedTicket(w.p, w.pmUser, "Client B rule", &w.b, w.secret)
	declined := e.seedTicket(w.p, w.pmUser, "Declined for Client A", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Still open", &w.a, w.ot)
	for _, tk := range []db.Ticket{core, forA, forB} {
		e.seedClose(tk, done, au, "Decided: "+tk.Title)
	}
	e.seedClose(declined, cancelled, au, "Declined: multi-level approval")
	path := fmt.Sprintf("/nodes/%d/behaviors", w.hr.ID)
	for c, want := range map[*http.Client][]string{admin: {core.Key, forA.Key, forB.Key}, w.pm: {core.Key, forA.Key}} {
		var list httpapi.BehaviorList
		if code := e.call(c, http.MethodGet, path, nil, &list); code != http.StatusOK {
			t.Fatalf("behaviors: %d", code)
		}
		var keys []string
		for _, b := range list.Items {
			keys = append(keys, b.Key)
		}
		if !slices.Equal(keys, want) {
			t.Errorf("behaviors: %v, want %v", keys, want)
		}
	}
}

// MSL-13: a done ticket without a confirmed decision record, such as an
// imported one, shows under its client marked unconfirmed, with its title as
// the change and its reason as the why.
func TestBehaviorsShowUnconfirmedHistory(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("hana@example.com", false)
	e.seedMember(au, w.p, "admin")
	done := statusID(e, admin, "Done")
	imported := e.seedTicket(w.p, w.pmUser, "Delivery notes hide prices", &w.a, w.ot)
	ctx := context.Background()
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET reason = 'Store staff must not see prices.' WHERE id = $1", imported.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: imported.ID, StatusID: done, Closed: true}); err != nil {
		t.Fatal(err)
	}
	confirmed := e.seedTicket(w.p, w.pmUser, "Overtime cap", &w.a, w.ot)
	e.seedClose(confirmed, done, au, "Overtime is capped at 40 hours.")
	var list httpapi.BehaviorList
	if code := e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/behaviors", w.ot.ID), nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("behaviors: %d %+v", code, list.Items)
	}
	for _, b := range list.Items {
		switch b.Key {
		case imported.Key:
			if !b.Unconfirmed || b.WhatChanged != "Delivery notes hide prices" || b.Why != "Store staff must not see prices." {
				t.Errorf("imported: %+v", b)
			}
		case confirmed.Key:
			if b.Unconfirmed || b.WhatChanged != "Overtime is capped at 40 hours." {
				t.Errorf("confirmed: %+v", b)
			}
		}
	}
}
