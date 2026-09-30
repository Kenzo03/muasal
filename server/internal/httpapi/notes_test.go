package httpapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func noteBody(title, decided string, client *int64, nodes []int64, tickets ...string) map[string]any {
	b := map[string]any{
		"title": title, "decided_on": decided, "attendees": "Hana, Budi (HR Manager)", "node_ids": nodes, "ticket_keys": tickets,
		"body": "## Decision\nHR approves overtime directly.\n\n## Why\nSupervisors are often on leave.\n\n## Alternatives rejected\nA backup supervisor.",
	}
	if client != nil {
		b["client_id"] = *client
	}
	return b
}

// AC-DC-8 at API level: a member records a note on Overtime Approval dated
// 2025-11-04; it gets key HRIS-DN1, appears on that node's timeline at that
// date and in search, and its history keeps every edit.
func TestMembersRecordDecisionNotes(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)

	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/notes", map[string]any{"title": "Hi", "decided_on": "2025-11-04", "node_ids": []int64{}, "body": ""}, &p); code != http.StatusUnprocessableEntity {
		t.Fatalf("empty note: %d %+v", code, p)
	}
	var fields []string
	for _, f := range *p.Errors {
		fields = append(fields, f.Field+":"+f.Code)
	}
	if fmt.Sprint(fields) != "[title:invalid body:required node_ids:min_items]" {
		t.Fatalf("field errors: %v", fields)
	}

	var n httpapi.Note
	code := e.call(w.pm, http.MethodPost, "/projects/HRIS/notes", noteBody("Overtime approval skips the supervisor", "2025-11-04", &w.a.ID, []int64{w.ot.ID}, tk.Key), &n)
	if code != http.StatusCreated || n.Key != "HRIS-DN1" || n.DecidedOn.String() != "2025-11-04" || n.Client == nil || n.Client.Name != "Client A" ||
		len(n.Nodes) != 1 || len(n.Tickets) != 1 || n.Tickets[0].Key != tk.Key || n.Author.Id != w.pmUser.ID || !n.CanEdit {
		t.Fatalf("create: %d %+v", code, n)
	}

	var tl httpapi.TimelinePage
	e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline", w.hr.ID), nil, &tl)
	if len(tl.Notes) != 1 || tl.Notes[0].Key != "HRIS-DN1" || tl.Notes[0].DecidedOn.String() != "2025-11-04" {
		t.Fatalf("timeline notes: %+v", tl.Notes)
	}
	e.call(w.pm, http.MethodGet, fmt.Sprintf("/nodes/%d/timeline?to=2025-11-03", w.ot.ID), nil, &tl)
	if len(tl.Notes) != 0 {
		t.Fatalf("a note outside the dates: %+v", tl.Notes)
	}
	var found httpapi.SearchResults
	if e.call(w.pm, http.MethodGet, "/search?q=hris-dn1", nil, &found); len(found.Notes) != 1 || found.Notes[0].Key != "HRIS-DN1" {
		t.Fatalf("search: %+v", found.Notes)
	}

	body := noteBody("Overtime approval goes straight to HR", "2025-11-04", &w.a.ID, []int64{w.ot.ID})
	var edited httpapi.Note
	if code := e.call(w.pm, http.MethodPatch, "/notes/HRIS-DN1", body, &edited); code != http.StatusOK || edited.Title != "Overtime approval goes straight to HR" || len(edited.Tickets) != 0 {
		t.Fatalf("edit: %d %+v", code, edited)
	}
	events, err := e.q.ListAuditEvents(t.Context(), db.ListAuditEventsParams{Entity: "note", EntityID: n.Id})
	if err != nil || len(events) != 2 || events[1].Action != "update" {
		t.Fatalf("history: %v %+v", err, events)
	}
	var list httpapi.NoteList
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/notes", nil, &list); len(list.Items) != 1 {
		t.Fatalf("list: %+v", list)
	}
}

// R-AC-2, R-AC-3 and §9.4: a Client B note is hidden from a member scoped to
// Client A; only the author and project admins edit a note.
func TestNotesFollowVisibilityAndAuthorship(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	var secret httpapi.Note
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/notes", noteBody("Client B report layout", "2025-10-01", &w.b.ID, []int64{w.secret.ID}), &secret); code != http.StatusCreated {
		t.Fatalf("admin note: %d", code)
	}
	if code := e.call(w.pm, http.MethodGet, "/notes/"+secret.Key, nil, nil); code != http.StatusNotFound {
		t.Fatalf("hidden note: %d", code)
	}
	var list httpapi.NoteList
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/notes", nil, &list); len(list.Items) != 0 {
		t.Fatalf("list leaks: %+v", list.Items)
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/notes", noteBody("Client B report layout", "2025-10-01", &w.b.ID, []int64{w.ot.ID}), &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "client_id" {
		t.Fatalf("a client outside the scope: %d %+v", code, p)
	}

	var core httpapi.Note
	e.call(admin, http.MethodPost, "/projects/HRIS/notes", noteBody("Overtime rounding for everyone", "2025-10-02", nil, []int64{w.ot.ID}), &core)
	var got httpapi.Note
	if e.call(w.pm, http.MethodGet, "/notes/"+core.Key, nil, &got); got.CanEdit {
		t.Fatalf("a member may edit an admin's note: %+v", got)
	}
	if code := e.call(w.pm, http.MethodPatch, "/notes/"+core.Key, noteBody("Overtime rounding changed", "2025-10-02", nil, []int64{w.ot.ID}), &p); code != http.StatusForbidden {
		t.Fatalf("member edit: %d", code)
	}
	archive := noteBody("Overtime rounding for everyone", "2025-10-02", nil, []int64{w.ot.ID})
	archive["archived"] = true
	if code := e.call(admin, http.MethodPatch, "/notes/"+core.Key, archive, &got); code != http.StatusOK || !got.Archived {
		t.Fatalf("archive: %d %+v", code, got)
	}
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/notes", nil, &list); len(list.Items) != 0 {
		t.Fatalf("an archived note is listed: %+v", list.Items)
	}
}

// MSL-11: a ticket filed from a note's action item joins the note's tickets;
// a note of another project, or none, is refused.
func TestTicketsFiledFromANoteJoinIt(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var n httpapi.Note
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/notes", noteBody("Monthly review with Client A", "2026-09-22", &w.a.ID, []int64{w.ot.ID}), &n); code != http.StatusCreated {
		t.Fatalf("note: %d", code)
	}
	body := map[string]any{"type": "change_request", "title": "Design the priority flag", "client_id": w.a.ID, "node_ids": []int64{w.ot.ID},
		"reason": "Action item of the monthly review: priority customers.", "note_key": strings.ToLower(n.Key)}
	var tk httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated {
		t.Fatalf("ticket: %d", code)
	}
	e.call(w.pm, http.MethodGet, "/notes/"+n.Key, nil, &n)
	if len(n.Tickets) != 1 || n.Tickets[0].Key != tk.Key {
		t.Fatalf("note tickets: %+v", n.Tickets)
	}
	body["note_key"] = "HRIS-DN99"
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "note_key" {
		t.Fatalf("unknown note: %d %+v", code, p)
	}
}
