package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/httpapi"
	"time"
)

// assign gives a ticket to a user, due in dueIn days from today on the test
// users' calendar, Asia/Jakarta (nil: no due date), with this reason.
func (e *env) assign(tk db.Ticket, to db.User, dueIn *int, reason string) {
	e.t.Helper()
	_, err := e.d.Pool.Exec(context.Background(),
		"UPDATE tickets SET assignee_id = $2, due_date = (now() AT TIME ZONE 'Asia/Jakarta')::date + $3::int, reason = $4 WHERE id = $1", tk.ID, to.ID, dueIn, reason)
	if err != nil {
		e.t.Fatal(err)
	}
}

func days(n int) *int { return &n }

func myKeys(p httpapi.MyTicketsPage) []string {
	out := []string{}
	for _, it := range p.Items {
		out = append(out, it.Key)
	}
	return out
}

// FSD §6.4: My tickets are the caller's open tickets that they may see,
// earliest due date first and undated last; the tabs narrow them, and the
// counts cover all of them whatever tab is shown.
func TestHomeListsMyOpenTicketsByDueDate(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	reason := "Client A supervisors are often on leave."
	later := e.seedTicket(w.p, w.pmUser, "Due in five days", &w.a, w.ot)
	e.assign(later, w.pmUser, days(5), reason)
	overdue := e.seedTicket(w.p, w.pmUser, "Due yesterday", &w.a, w.ot)
	e.assign(overdue, w.pmUser, days(-1), reason)
	undated := e.seedTicket(w.p, w.pmUser, "No due date and no menu", &w.a)
	e.assign(undated, w.pmUser, nil, reason)
	noReason := e.seedTicket(w.p, w.pmUser, "Due in ten days, no reason", nil, w.ot)
	e.assign(noReason, w.pmUser, days(10), "")
	closed := e.seedTicket(w.p, w.pmUser, "Already done", &w.a, w.ot)
	e.assign(closed, w.pmUser, days(1), reason)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: closed.ID, StatusID: statusID(e, w.pm, "Done"), Closed: true}); err != nil {
		t.Fatal(err)
	}
	hidden := e.seedTicket(w.p, w.pmUser, "Client B work", &w.b, w.secret) // the PM sees Client A only
	e.assign(hidden, w.pmUser, days(2), reason)
	_, ani := e.signedIn("ani@example.com", false)
	e.seedMember(ani, w.p, "member")
	e.assign(e.seedTicket(w.p, w.pmUser, "Ani's work", &w.a, w.ot), ani, days(3), reason)

	var page httpapi.MyTicketsPage
	if code := e.call(w.pm, http.MethodGet, "/me/tickets", nil, &page); code != http.StatusOK {
		t.Fatalf("my tickets: %d", code)
	}
	if got, want := myKeys(page), []string{overdue.Key, later.Key, noReason.Key, undated.Key}; !slices.Equal(got, want) {
		t.Fatalf("order: %v, want %v", got, want)
	}
	if c := page.Counts; c.All != 4 || c.Overdue != 1 || c.Week != 1 || c.Incomplete != 2 {
		t.Fatalf("counts: %+v", c)
	}
	if len(page.Projects) != 1 || page.Projects[0].Key != "HRIS" || page.Projects[0].Open != 4 {
		t.Fatalf("projects: %+v", page.Projects)
	}
	first, last := page.Items[0], page.Items[3]
	if first.Menu == nil || *first.Menu != "HR › Overtime Approval" || first.Status.Name != "To do" || first.MissingReason || first.MissingMenus ||
		first.Client == nil || first.Client.Name != "Client A" || first.DueDate == nil {
		t.Fatalf("the first row: %+v", first)
	}
	if last.Menu != nil || !last.MissingMenus || last.DueDate != nil {
		t.Fatalf("the undated row: %+v", last)
	}
	for view, want := range map[string][]string{
		"overdue":    {overdue.Key},
		"week":       {later.Key},
		"incomplete": {noReason.Key, undated.Key},
	} {
		var p httpapi.MyTicketsPage
		e.call(w.pm, http.MethodGet, "/me/tickets?view="+view, nil, &p)
		if got := myKeys(p); !slices.Equal(got, want) || p.Counts.All != 4 {
			t.Errorf("view %s: %v %+v, want %v", view, got, p.Counts, want)
		}
	}
	if code := e.call(w.pm, http.MethodGet, "/me/tickets?view=someday", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("an unknown view: %d", code)
	}
}

// FSD §6.4: Recently updated lists the tickets the caller may see that changed
// last, newest first, each with its latest change: an event or a comment.
func TestHomeListsRecentChangesInScope(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	create := func(title string) httpapi.Ticket {
		t.Helper()
		var tk httpapi.Ticket
		body := map[string]any{"title": title, "type": "bug", "client_id": w.a.ID, "requester_contact_id": w.budi, "node_ids": []int64{w.ot.ID}}
		if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated {
			t.Fatalf("create %q: %d", title, code)
		}
		return tk
	}
	untouched, moved, commented := create("Created only"), create("Moved to In progress"), create("Commented on")
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+moved.Key+"/transition", map[string]any{"status_id": statusID(e, w.pm, "In progress")}, nil); code != http.StatusOK {
		t.Fatalf("move: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+commented.Key+"/comments", map[string]any{"body": "Budi confirmed by phone."}, nil); code != http.StatusCreated {
		t.Fatalf("comment: %d", code)
	}
	hidden := e.seedTicket(w.p, w.pmUser, "Client B work", &w.b, w.secret) // changed last, but the PM sees Client A only
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET updated_at = now() + interval '1 hour' WHERE id = $1", hidden.ID); err != nil {
		t.Fatal(err)
	}

	var list httpapi.RecentTicketList
	if code := e.call(w.pm, http.MethodGet, "/me/updates", nil, &list); code != http.StatusOK {
		t.Fatalf("updates: %d", code)
	}
	var got []string
	for _, it := range list.Items {
		if it.Change == nil {
			t.Fatalf("%s has no change", it.Key)
		}
		got = append(got, it.Key+" "+string(it.Change.Kind)+" "+orBlank(it.Change.Action))
	}
	want := []string{commented.Key + " comment ", moved.Key + " event transition", untouched.Key + " event create"}
	if !slices.Equal(got, want) {
		t.Fatalf("updates: %q, want %q", got, want)
	}
	if c := list.Items[0].Change; c.Actor == nil || c.Actor.Id != w.pmUser.ID || c.Body != nil || list.Items[1].Status.Name != "In progress" {
		t.Fatalf("the comment change: %+v", c)
	}
}

// orBlank is a pointer's text, or "" for nil.
func orBlank(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MSL-9: a project admin with nothing assigned still sees what is late,
// unowned or incomplete in their projects; a member does not.
func TestHomeNeedsAttentionForProjectAdmins(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	late := e.seedTicket(w.p, w.pmUser, "Delivery note prints the wrong warehouse", &w.a, w.ot)
	e.assign(late, w.pmUser, days(-3), "Drivers went to the wrong warehouse twice.")
	unowned := e.seedTicket(w.p, w.pmUser, "Invoice terms of 45 days", nil, w.ot)
	if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET reason = 'Permintaan klien.' WHERE id = $1", unowned.ID); err != nil {
		t.Fatal(err)
	}
	var got httpapi.AttentionList
	if code := e.call(lead, http.MethodGet, "/me/attention", nil, &got); code != http.StatusOK || len(got.Items) != 1 {
		t.Fatalf("lead: %d %+v", code, got)
	}
	if a := got.Items[0]; a.Key != "HRIS" || a.Overdue != 1 || a.Unassigned != 1 || a.WeakReason != 1 || a.NoReason != 0 || a.NoMenu != 0 || a.Stale != 0 {
		t.Fatalf("counts: %+v", a)
	}
	if code := e.call(w.pm, http.MethodGet, "/me/attention", nil, &got); code != http.StatusOK || len(got.Items) != 0 {
		t.Fatalf("a member: %d %+v", code, got)
	}
}

// MSL-18: a fresh server's checklist starts empty and ticks off as the admin
// chooses AI, invites someone, and makes a project, its tree and tickets.
func TestSetupChecklist(t *testing.T) {
	e := newEnv(t)
	admin, au := e.signedIn("admin@example.com", true)
	type status struct {
		AI, Invited, Project, Tree, History bool
		FirstProject                        string `json:"first_project"`
	}
	var st status
	if code := e.call(admin, http.MethodGet, "/admin/setup", nil, &st); code != http.StatusOK || st != (status{}) {
		t.Fatalf("fresh: %d %+v", code, st)
	}
	p := e.seedProject("HRIS")
	node := e.seedNode(p, nil, "module", "HR")
	e.seedTicket(p, au, "First request", nil, node)
	member, mu := e.signedIn("budi@example.com", false)
	e.seedMember(mu, p, "member")
	e.localAI(admin)
	if e.call(admin, http.MethodGet, "/admin/setup", nil, &st); st != (status{true, true, true, true, true, "HRIS"}) {
		t.Fatalf("set up: %+v", st)
	}
	if code := e.call(member, http.MethodGet, "/admin/setup", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member: %d", code)
	}
}

// MSL-48: a project admin sees how far the project is set up; members don't.
func TestProjectSetupChecklist(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "HRIS"}, nil); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	var st httpapi.ProjectSetup
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/setup", nil, &st); code != http.StatusOK ||
		st.Clients || st.Team || st.Documents || st.Tree || st.Tickets || st.Repos {
		t.Fatalf("a new project: %d %+v", code, st)
	}
	p, err := e.q.GetProjectByKey(t.Context(), "HRIS")
	if err != nil {
		t.Fatal(err)
	}
	a := e.seedClient("Client A")
	if _, err := e.d.Pool.Exec(t.Context(), `INSERT INTO project_clients (project_id, client_id) VALUES ($1, $2)`, p.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member")
	e.seedTicket(p, mu, "First request", &a)
	if e.call(admin, http.MethodGet, "/projects/HRIS/setup", nil, &st); !st.Clients || !st.Team || !st.Tickets || st.Tree || st.Documents {
		t.Fatalf("after clients, team and a ticket: %+v", st)
	}
	if code := e.call(member, http.MethodGet, "/projects/HRIS/setup", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member reads the checklist: %d", code)
	}
}

// MSL-52: from 08:00 in the assignee's timezone they hear, once a day, about
// tickets due today or tomorrow, or overdue since yesterday; nothing else.
func TestDueReminders(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	due := func(title, day string, assignee *db.User) db.Ticket {
		tk := e.seedTicket(w.p, w.pmUser, title, &w.a, w.ot)
		if _, err := e.d.Pool.Exec(t.Context(), `UPDATE tickets SET due_date = $2, assignee_id = $3 WHERE id = $1`, tk.ID, day, assignee.ID); err != nil {
			t.Fatal(err)
		}
		return tk
	}
	due("Due today", "2026-09-30", &w.pmUser)
	due("Due tomorrow", "2026-10-01", &w.pmUser)
	due("Overdue since yesterday", "2026-09-29", &w.pmUser)
	due("Overdue for a week", "2026-09-23", &w.pmUser)
	due("Due next week", "2026-10-07", &w.pmUser)
	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	if n, err := e.api.RemindDue(t.Context(), time.Date(2026, 9, 30, 7, 30, 0, 0, jakarta)); err != nil || n != 0 {
		t.Fatalf("before 08:00: %d %v", n, err)
	}
	morning := time.Date(2026, 9, 30, 8, 5, 0, 0, jakarta)
	if n, err := e.api.RemindDue(t.Context(), morning); err != nil || n != 3 {
		t.Fatalf("at 08:05: %d %v", n, err)
	}
	if n, _ := e.api.RemindDue(t.Context(), morning.Add(time.Hour)); n != 0 {
		t.Fatalf("an hour later, again: %d", n)
	}
	var got httpapi.NotificationList
	e.call(w.pm, http.MethodGet, "/notifications", nil, &got)
	whens := map[string]bool{}
	for _, n := range got.Items {
		if n.Type == httpapi.NotificationTypeDue {
			whens[n.Payload["when"].(string)] = true
		}
	}
	if len(whens) != 3 || !whens["today"] || !whens["tomorrow"] || !whens["overdue"] {
		t.Fatalf("reminders: %+v", got.Items)
	}
}
