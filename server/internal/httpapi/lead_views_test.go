package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// exec runs SQL a test needs to set up, such as backdating a change.
func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.d.Pool.Exec(context.Background(), sql, args...); err != nil {
		e.t.Fatal(err)
	}
}

func sortedKeys(p httpapi.TicketPage) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, it.Key)
	}
	slices.Sort(out)
	return out
}

// A project lead filters the list by owner (anyone, or nobody), by due date
// as Home counts it, and by staleness; due and stale count open tickets only.
func TestTicketListFiltersByOwnerDueDateAndStaleness(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	reason := "Client A asked for it."
	_, ani := e.signedIn("ani@example.com", false)
	e.seedMember(ani, w.p, "member")
	overdue := e.seedTicket(w.p, w.pmUser, "Overdue, the PM's", &w.a, w.ot)
	e.assign(overdue, w.pmUser, days(-2), reason)
	week := e.seedTicket(w.p, w.pmUser, "Due this week, Ani's", &w.a, w.ot)
	e.assign(week, ani, days(3), reason)
	nobody := e.seedTicket(w.p, w.pmUser, "Nobody owns this", &w.a, w.ot)
	stale := e.seedTicket(w.p, w.pmUser, "Unchanged for ten days", nil, w.ot)
	e.assign(stale, ani, nil, reason)
	e.exec("UPDATE tickets SET updated_at = now() - interval '10 days' WHERE id = $1", stale.ID)
	closed := e.seedTicket(w.p, w.pmUser, "Closed, overdue and old", &w.a, w.ot)
	e.assign(closed, ani, days(-5), reason)
	e.seedClose(closed, statusID(e, w.pm, "Done"), w.pmUser, "Done.")
	e.exec("UPDATE tickets SET updated_at = now() - interval '30 days' WHERE id = $1", closed.ID)

	for query, want := range map[string][]string{
		fmt.Sprintf("assignee_id=%d", ani.ID): {week.Key, stale.Key, closed.Key},
		"unassigned=true":                     {nobody.Key},
		"due=overdue":                         {overdue.Key},
		"due=week":                            {week.Key},
		"stale_days=7":                        {stale.Key},
		fmt.Sprintf("assignee_id=%d&stale_days=7", ani.ID): {stale.Key},
	} {
		var page httpapi.TicketPage
		if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?"+query, nil, &page); code != http.StatusOK {
			t.Fatalf("%s: %d", query, code)
		}
		slices.Sort(want)
		if got := sortedKeys(page); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
	if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?stale_days=0", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("stale_days=0: %d", code)
	}
}

// The workload page counts each person's open tickets the caller may see:
// a member without tickets still has a row, viewers have none, and
// unassigned work comes last.
func TestWorkloadCountsOpenTicketsPerPerson(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	reason := "Client A asked for it."
	_, ani := e.signedIn("ani@example.com", false)
	e.seedMember(ani, w.p, "member")
	_, idle := e.signedIn("idle@example.com", false)
	e.seedMember(idle, w.p, "member")
	_, viewer := e.signedIn("viewer@example.com", false)
	e.seedMember(viewer, w.p, "viewer")
	e.assign(e.seedTicket(w.p, w.pmUser, "Overdue", &w.a, w.ot), ani, days(-1), reason)
	week := e.seedTicket(w.p, w.pmUser, "Urgent, due this week, untouched", &w.a, w.ot)
	e.assign(week, ani, days(2), reason)
	e.exec("UPDATE tickets SET priority = 'urgent', updated_at = now() - interval '9 days' WHERE id = $1", week.ID)
	moving := e.seedTicket(w.p, w.pmUser, "In progress, core work", nil, w.ot)
	e.assign(moving, ani, nil, reason)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: moving.ID, StatusID: statusID(e, w.pm, "In progress")}); err != nil {
		t.Fatal(err)
	}
	done := e.seedTicket(w.p, w.pmUser, "Done", &w.a, w.ot)
	e.assign(done, ani, days(-3), reason)
	e.seedClose(done, statusID(e, w.pm, "Done"), w.pmUser, "Done.")
	e.assign(e.seedTicket(w.p, w.pmUser, "Client B work", &w.b, w.secret), ani, days(-1), reason) // the PM sees Client A only
	e.seedTicket(w.p, w.pmUser, "Nobody's", &w.a, w.ot)

	var wl httpapi.Workload
	if code := e.call(w.pm, http.MethodGet, "/projects/HRIS/workload", nil, &wl); code != http.StatusOK || wl.StaleDays != 7 {
		t.Fatalf("workload: %d %+v", code, wl)
	}
	var got []string
	for _, r := range wl.Rows {
		who := "nobody"
		if r.Assignee != nil {
			who = r.Assignee.Name
		}
		got = append(got, fmt.Sprintf("%s open=%d progress=%d overdue=%d week=%d stale=%d high=%d", who, r.Open, r.InProgress, r.Overdue, r.DueWeek, r.Stale, r.High))
	}
	want := []string{
		"ani@example.com open=3 progress=1 overdue=1 week=1 stale=1 high=1",
		"idle@example.com open=0 progress=0 overdue=0 week=0 stale=0 high=0",
		"pm@example.com open=0 progress=0 overdue=0 week=0 stale=0 high=0",
		"nobody open=1 progress=0 overdue=0 week=0 stale=0 high=0",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A new project can copy a template's statuses and live module tree:
// client-specific menus arrive shared and archived nodes stay behind.
func TestNewProjectCopiesATemplate(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	admin, _ := e.signedIn("admin@example.com", true)
	if _, err := e.q.InsertStatus(ctx, db.InsertStatusParams{ProjectID: w.p.ID, Name: "Waiting on client", Category: "in_progress", Position: 9, Color: "#8A6D3B"}); err != nil {
		t.Fatal(err)
	}
	e.exec("UPDATE nodes SET archived_at = now() WHERE id = $1", e.seedNode(w.p, &w.hr, "menu", "Old Report").ID)

	var p httpapi.Project
	if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "PAY", "name": "Payroll", "template_key": "hris"}, &p); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	names := func(projectID int64) []string {
		rows, err := e.q.ListStatuses(ctx, projectID)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, s := range rows {
			out = append(out, fmt.Sprintf("%s/%s/%v", s.Name, s.Category, s.IsDefault))
		}
		return out
	}
	if got, want := names(p.Id), names(w.p.ID); !slices.Equal(got, want) || !slices.Contains(got, "Waiting on client/in_progress/false") {
		t.Fatalf("statuses: %v, want %v", got, want)
	}
	nodes, err := e.q.ListNodes(ctx, db.ListNodesParams{ProjectID: p.Id, AllClients: true, ClientIds: []int64{}})
	if err != nil {
		t.Fatal(err)
	}
	var tree []string
	for _, n := range nodes {
		tree = append(tree, fmt.Sprintf("%s %s shared=%v top=%v", n.Type, n.Name, !n.ClientSpecific, n.ParentID == nil))
	}
	want := []string{"module HR shared=true top=true", "menu Overtime Approval shared=true top=false", "menu Client B Report shared=true top=false"}
	if !slices.Equal(tree, want) {
		t.Fatalf("tree: %v, want %v", tree, want)
	}
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "OPS", "name": "Ops", "template_key": "NOPE"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "template_key" {
		t.Fatalf("unknown template: %d %+v", code, prob)
	}
}

// A weekly summary runs on its weekday for the past seven days, once a day,
// for its client with core work; with AI off it lists each change from its
// decision record. Only project admins schedule and stop one.
func TestWeeklySummaryListsTheWeeksDecisions(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	now := time.Now().UTC()
	weekday := (int(now.Weekday())+6)%7 + 1
	body := map[string]any{"client_id": w.a.ID, "language": "en", "audience": "client", "weekday": weekday}

	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/summary-schedules", body, nil); code != http.StatusForbidden {
		t.Fatalf("a member schedules: %d", code)
	}
	var prob httpapi.Problem
	unlinked := e.seedClient("Client C")
	if code := e.call(lead, http.MethodPost, "/projects/HRIS/summary-schedules", map[string]any{"client_id": unlinked.ID, "language": "en", "audience": "client", "weekday": weekday}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Field != "client_id" {
		t.Fatalf("an unlinked client: %d %+v", code, prob)
	}
	var sch httpapi.SummarySchedule
	if code := e.call(lead, http.MethodPost, "/projects/HRIS/summary-schedules", body, &sch); code != http.StatusCreated || sch.Client == nil || sch.Client.Name != "Client A" {
		t.Fatalf("schedule: %d %+v", code, sch)
	}

	done := statusID(e, w.pm, "Done")
	a := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	e.seedClose(a, done, w.pmUser, "HR approves overtime directly.")
	b := e.seedTicket(w.p, w.pmUser, "Client B report", &w.b, w.secret)
	e.seedClose(b, done, w.pmUser, "Client B's report changed.")
	e.exec("UPDATE tickets SET closed_at = now() - interval '2 days' WHERE id = ANY ($1)", []int64{a.ID, b.ID})
	e.seedClose(e.seedTicket(w.p, w.pmUser, "Closed today", &w.a, w.ot), done, w.pmUser, "Belongs to next week.")

	for range 2 { // the second run, the same day, writes nothing
		if err := e.api.RunSummarySchedules(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var list httpapi.SummaryList
	if code := e.call(lead, http.MethodGet, "/summaries?project=HRIS", nil, &list); code != http.StatusOK || len(list.Items) != 1 {
		t.Fatalf("summaries: %d %+v", code, list)
	}
	var sum httpapi.Summary
	e.call(lead, http.MethodGet, fmt.Sprintf("/summaries/%d", list.Items[0].Id), nil, &sum)
	if !strings.HasPrefix(sum.Title, "HRIS changes for Client A") || !strings.Contains(sum.Markdown, "HR approves overtime directly.") ||
		strings.Contains(sum.Markdown, "Client B's report") || strings.Contains(sum.Markdown, "next week") || sum.Model != "" || sum.Scope.Audience != httpapi.SummaryAudienceClient {
		t.Fatalf("summary: %+v", sum)
	}
	var notes httpapi.NotificationList
	e.call(lead, http.MethodGet, "/notifications", nil, &notes)
	if len(notes.Items) != 1 || notes.Items[0].Type != "job_done" || notes.Items[0].Payload["link"] != fmt.Sprintf("/summaries/%d", sum.Id) {
		t.Fatalf("notifications: %+v", notes.Items)
	}
	var schedules httpapi.SummaryScheduleList
	e.call(lead, http.MethodGet, "/projects/HRIS/summary-schedules", nil, &schedules)
	if len(schedules.Items) != 1 || schedules.Items[0].LastRunOn == nil || schedules.Items[0].LastRunOn.Format(time.DateOnly) != now.Format(time.DateOnly) {
		t.Fatalf("schedules: %+v", schedules.Items)
	}
	if code := e.call(w.pm, http.MethodDelete, fmt.Sprintf("/summary-schedules/%d", sch.Id), nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member stops it: %d", code)
	}
	if code := e.call(lead, http.MethodDelete, fmt.Sprintf("/summary-schedules/%d", sch.Id), nil, nil); code != http.StatusNoContent {
		t.Fatalf("stop: %d", code)
	}
}
