package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/httpapi"
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

// Overdue and "this week" count from the day on the user's own calendar.
// Kiritimati (UTC+14) is always a day or two ahead of Pago Pago (UTC-11), so a
// ticket due today in Pago Pago is already overdue in Kiritimati, whatever the
// hour on the server.
func TestDueDatesCountFromTheUsersDay(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	pago, err := time.LoadLocation("Pacific/Pago_Pago")
	if err != nil {
		t.Fatal(err)
	}
	tk := e.seedTicket(w.p, w.pmUser, "Due today in Pago Pago", &w.a, w.ot)
	e.exec("UPDATE tickets SET assignee_id = $2, due_date = $3::date, reason = 'Payroll closes that day.' WHERE id = $1",
		tk.ID, w.pmUser.ID, time.Now().In(pago).Format(time.DateOnly))
	for _, c := range []struct {
		tz            string
		overdue, week int
	}{{"Pacific/Kiritimati", 1, 0}, {"Pacific/Pago_Pago", 0, 1}} {
		e.exec("UPDATE users SET timezone = $2 WHERE id = $1", w.pmUser.ID, c.tz)
		var home httpapi.MyTicketsPage
		var list httpapi.TicketPage
		var wl httpapi.Workload
		for path, out := range map[string]any{"/me/tickets": &home, "/projects/HRIS/tickets?due=overdue": &list, "/projects/HRIS/workload": &wl} {
			if code := e.call(w.pm, http.MethodGet, path, nil, out); code != http.StatusOK {
				t.Fatalf("%s %s: %d", c.tz, path, code)
			}
		}
		listed := 0
		if slices.Contains(sortedKeys(list), tk.Key) {
			listed = 1
		}
		var pm httpapi.WorkloadRow
		for _, r := range wl.Rows {
			if r.Assignee != nil && r.Assignee.Name == w.pmUser.Name {
				pm = r
			}
		}
		got := fmt.Sprintf("home overdue=%d week=%d, list overdue=%d, workload overdue=%d week=%d", home.Counts.Overdue, home.Counts.Week, listed, pm.Overdue, pm.DueWeek)
		want := fmt.Sprintf("home overdue=%d week=%d, list overdue=%d, workload overdue=%d week=%d", c.overdue, c.week, c.overdue, c.overdue, c.week)
		if got != want {
			t.Errorf("%s: %s, want %s", c.tz, got, want)
		}
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

// MSL-54: tickets carry an optional estimate in hours; Workload adds up each
// person's open hours.
func TestEstimatesAddUpOnWorkload(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	file := func(title string, hours any) (int, httpapi.Ticket) {
		var tk httpapi.Ticket
		code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
			"type": "feature", "title": title, "client_id": w.a.ID, "node_ids": []int64{w.ot.ID}, "reason": "Needed for the pilot",
			"assignee_id": w.pmUser.ID, "estimate_hours": hours,
		}, &tk)
		return code, tk
	}
	if code, tk := file("Copy last week's shifts", 6.5); code != http.StatusCreated || tk.EstimateHours == nil || *tk.EstimateHours != 6.5 {
		t.Fatalf("create with an estimate: %d %+v", code, tk.EstimateHours)
	}
	file("Salary slips as PDF", 10)
	file("Holiday calendar", nil)
	if code, _ := file("Negative work", -1); code != http.StatusUnprocessableEntity {
		t.Fatalf("a negative estimate: %d", code)
	}
	var wl httpapi.Workload
	e.call(w.pm, http.MethodGet, "/projects/HRIS/workload", nil, &wl)
	found := false
	for _, r := range wl.Rows {
		if r.Assignee != nil && r.Assignee.Id == w.pmUser.ID {
			found = true
			if r.OpenHours != 16.5 || r.Estimated != 2 || r.Open != 3 {
				t.Fatalf("workload row: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("no workload row for the assignee: %+v", wl.Rows)
	}
}

// MSL-55: a description's task list shows as progress in the list.
func TestChecklistProgressInTheList(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var tk httpapi.Ticket
	desc := "Langkah:\n- [x] Salin jadwal\n- [ ] Ubah shift pagi\n  * [X] Kirim ke toko\n1. [ ] Uji di 10 toko\n\nBukan tugas: [ ] di tengah kalimat."
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "feature", "title": "Copy last week's shifts", "client_id": w.a.ID, "node_ids": []int64{w.ot.ID}, "description": desc,
	}, &tk); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	var page httpapi.TicketPage
	e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets", nil, &page)
	for _, it := range page.Items {
		if it.Key == tk.Key {
			if it.Checklist == nil || it.Checklist.Done != 2 || it.Checklist.Total != 4 {
				t.Fatalf("checklist: %+v", it.Checklist)
			}
			return
		}
	}
	t.Fatalf("ticket not listed: %+v", page.Items)
}

// MSL-56: labels are normalised, filter the list, and the project lists them.
func TestLabelsGroupTickets(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	file := func(title string, labels []string) (int, httpapi.Ticket) {
		var tk httpapi.Ticket
		code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
			"type": "feature", "title": title, "client_id": w.a.ID, "node_ids": []int64{w.ot.ID}, "labels": labels,
		}, &tk)
		return code, tk
	}
	if code, tk := file("Pilot accounts for 10 stores", []string{" Pilot ", "UAT", "pilot"}); code != http.StatusCreated ||
		tk.Labels == nil || strings.Join(*tk.Labels, ",") != "pilot,uat" {
		t.Fatalf("create: %d %+v", code, tk.Labels)
	}
	file("Migrate leave balances", []string{"migration", "pilot"})
	file("Holiday calendar", nil)
	if code, _ := file("Too many labels", []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}); code != http.StatusUnprocessableEntity {
		t.Fatalf("eleven labels: %d", code)
	}
	var page httpapi.TicketPage
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?label=PILOT", nil, &page); len(page.Items) != 2 {
		t.Fatalf("filter by label: %+v", page.Items)
	}
	var labels struct {
		Items []struct {
			Label string
			Uses  int
		}
	}
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/labels", nil, &labels); len(labels.Items) != 3 || labels.Items[0].Label != "pilot" || labels.Items[0].Uses != 2 {
		t.Fatalf("labels: %+v", labels.Items)
	}
}
