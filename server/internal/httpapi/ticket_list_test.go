package httpapi_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func ticketKeys(page httpapi.TicketPage) []string {
	keys := []string{}
	for _, t := range page.Items {
		keys = append(keys, t.Key)
	}
	return keys
}

func TestTicketListFiltersSortsAndScopes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	leave := e.seedNode(w.p, &w.hr, "menu", "Leave Request")
	t1 := e.seedTicket(w.p, au, "Overtime rules for Client A", &w.a, w.ot)  // HRIS-1
	t2 := e.seedTicket(w.p, au, "Leave balance on the payslip", nil, leave) // HRIS-2, core work
	e.seedTicket(w.p, au, "Client B payroll export", &w.b)                  // HRIS-3, hidden from the PM
	var st httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: t1.ID, StatusID: st.Items[1].Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.q.UpdateTicket(ctx, db.UpdateTicketParams{
		ID: t2.ID, Version: 1, Type: "feature", Title: t2.Title, Reason: "Employees ask about it every month.",
		RequesterUserID: &au.ID, AssigneeID: &w.pmUser.ID, Priority: "urgent",
	}); err != nil {
		t.Fatal(err)
	}
	list := func(c *http.Client, query string) []string {
		t.Helper()
		var page httpapi.TicketPage
		if code := e.call(c, http.MethodGet, "/projects/HRIS/tickets"+query, nil, &page); code != http.StatusOK {
			t.Fatalf("%s: %d", query, code)
		}
		return ticketKeys(page)
	}
	for query, want := range map[string][]string{
		"":                                   {"HRIS-2", "HRIS-1"}, // latest update first; HRIS-3 is out of scope
		"?sort=key":                          {"HRIS-1", "HRIS-2"},
		"?sort=priority":                     {"HRIS-2", "HRIS-1"},
		"?mine=true":                         {"HRIS-2"},
		"?core=true":                         {"HRIS-2"},
		fmt.Sprintf("?client_id=%d", w.a.ID): {"HRIS-1"},
		"?type=feature":                      {"HRIS-2"},
		fmt.Sprintf("?status_id=%d", st.Items[1].Id): {"HRIS-1"},
		"?category=todo":                    {"HRIS-2"},
		fmt.Sprintf("?node_id=%d", w.hr.ID): {"HRIS-2", "HRIS-1"}, // sub-nodes count (AC-MR-4)
		fmt.Sprintf("?node_id=%d", w.ot.ID): {"HRIS-1"},
		"?missing=reason":                   {"HRIS-1"},
		"?missing=menus":                    {},
		"?q=payslip":                        {"HRIS-2"},
		"?q=hris-1":                         {"HRIS-1"},
	} {
		if got := list(w.pm, query); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", query, got, want)
		}
	}
	if got := list(admin, ""); !slices.Equal(got, []string{"HRIS-2", "HRIS-1", "HRIS-3"}) {
		t.Errorf("admin: %v", got)
	}
	var page httpapi.TicketPage
	e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?sort=key", nil, &page)
	first, second := page.Items[0], page.Items[1]
	if first.Client == nil || first.Client.Name != "Client A" || !slices.Equal(first.NodeNames, []string{"Overtime Approval"}) ||
		!first.MissingReason || second.Assignee == nil || second.Assignee.Name != "pm@example.com" || second.MissingReason ||
		second.Priority != httpapi.PriorityUrgent {
		t.Fatalf("rows: %+v %+v", first, second)
	}
}

func TestTicketListPagesByCursor(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	for i := 1; i <= 5; i++ {
		e.seedTicket(p, au, fmt.Sprintf("Ticket number %d", i), nil)
	}
	var got [][]string
	path := "/projects/HRIS/tickets?sort=key&limit=2"
	for {
		var page httpapi.TicketPage
		if code := e.call(admin, http.MethodGet, path, nil, &page); code != http.StatusOK {
			t.Fatalf("page: %d", code)
		}
		got = append(got, ticketKeys(page))
		if page.NextCursor == nil {
			break
		}
		path = "/projects/HRIS/tickets?sort=key&limit=2&cursor=" + *page.NextCursor
	}
	if fmt.Sprint(got) != "[[HRIS-1 HRIS-2] [HRIS-3 HRIS-4] [HRIS-5]]" {
		t.Fatalf("pages: %v", got)
	}
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/tickets?cursor=nope", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("bad cursor: %d", code)
	}
}

// FSD §8.4: the board asks only for tickets closed in the last 14 days.
func TestTicketListCanLeaveOutOldCloses(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	done := statusID(e, w.pm, "Done")
	old := e.seedTicket(w.p, w.pmUser, "Closed long ago", &w.a, w.ot)
	recent := e.seedTicket(w.p, w.pmUser, "Closed yesterday", &w.a, w.ot)
	open := e.seedTicket(w.p, w.pmUser, "Still open", &w.a, w.ot)
	for _, tk := range []db.Ticket{old, recent} {
		if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: done, Closed: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET closed_at = now() - interval '20 days' WHERE id = $1", old.ID); err != nil {
		t.Fatal(err)
	}
	for query, want := range map[string][]string{
		"?sort=key":                {old.Key, recent.Key, open.Key},
		"?sort=key&closed_days=14": {recent.Key, open.Key},
	} {
		var page httpapi.TicketPage
		e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets"+query, nil, &page)
		if got := ticketKeys(page); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
	}
}

// §8.5: the list's filter downloads as CSV, only with rows the member may
// see, and a title that looks like a formula stays text.
func TestTicketListExportsCSV(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	e.seedTicket(w.p, w.pmUser, "=HYPERLINK(\"http://evil\")", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Overtime export", nil, w.ot)
	e.seedTicket(w.p, w.pmUser, "Client B only", &w.b)
	e.exec("UPDATE statuses SET name = '=' || name WHERE project_id = $1", w.p.ID)
	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/projects/HRIS/tickets?format=csv&sort=key", nil)
	res, err := w.pm.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/csv") ||
		!strings.Contains(res.Header.Get("Content-Disposition"), "HRIS-tickets-") {
		t.Fatalf("export: %d %v", res.StatusCode, res.Header)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 3 || rows[0][0] != "key" {
		t.Fatalf("rows: %v %q", err, body)
	}
	for _, r := range rows[1:] {
		if r[3] != "'=To do" {
			t.Fatalf("status cell: %q", r[3])
		}
		if strings.Contains(r[1], "Client B") {
			t.Fatalf("a Client B ticket leaked: %v", r)
		}
		for _, cell := range r {
			if strings.HasPrefix(cell, "=") {
				t.Fatalf("a formula cell: %v", r)
			}
		}
	}
}

// MSL-12: tickets filed through the API or an import never show the form's
// weak-reason hint, so the list finds them by the same rule (R-DC-8). An
// empty reason is missing, not weak.
func TestTicketListFindsWeakReasons(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, au := e.signedIn("admin@example.com", true)
	for reason, title := range map[string]string{
		"Permintaan klien.": "Invoice terms of 45 days",
		"The client's auditor requires a 1% stock tolerance from 2027.": "Stock tolerance of 1%",
		"": "No reason yet",
	} {
		tk := e.seedTicket(w.p, au, title, nil, w.ot)
		if _, err := e.d.Pool.Exec(context.Background(), "UPDATE tickets SET reason = $1 WHERE id = $2", reason, tk.ID); err != nil {
			t.Fatal(err)
		}
	}
	var page httpapi.TicketPage
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/tickets?missing=weak_reason", nil, &page); code != http.StatusOK ||
		len(page.Items) != 1 || page.Items[0].Title != "Invoice terms of 45 days" {
		t.Fatalf("%d %+v", code, page.Items)
	}
}
