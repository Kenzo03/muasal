package httpapi_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func statusNames(list httpapi.StatusList) []string {
	names := make([]string, len(list.Items))
	for i, s := range list.Items {
		names[i] = s.Name
	}
	return names
}

func statusInput(s httpapi.Status) map[string]any {
	return map[string]any{"id": s.Id, "name": s.Name, "category": s.Category, "color": s.Color, "is_default": s.IsDefault}
}

func TestNewProjectsStartWithTheDefaultStatuses(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "HRIS"}, nil)
	var list httpapi.StatusList
	if code := e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list); code != http.StatusOK ||
		!slices.Equal(statusNames(list), []string{"To do", "In progress", "In review", "Done", "Cancelled"}) || !list.Items[0].IsDefault {
		t.Fatalf("statuses: %d %+v", code, list)
	}
}

func TestAdminEditsStatusesAndMovesTicketsOffRemovedOnes(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p := e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	var list httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
	todo, prog, review, done, cancelled := list.Items[0], list.Items[1], list.Items[2], list.Items[3], list.Items[4]
	tk := e.seedTicket(p, au, "Waiting for review", nil)
	if _, err := e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: review.Id}); err != nil {
		t.Fatal(err)
	}
	var prob httpapi.Problem
	body := map[string]any{"statuses": []map[string]any{statusInput(todo), statusInput(prog), statusInput(done), statusInput(cancelled)}}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", body, &prob); code != http.StatusConflict || prob.Code != "status_in_use" {
		t.Fatalf("remove a status in use: %d %+v", code, prob)
	}
	shipped := statusInput(done)
	shipped["name"] = "Shipped"
	body = map[string]any{
		"statuses": []map[string]any{statusInput(todo), {"name": "Blocked", "category": "in_progress", "color": "#DC2626"},
			statusInput(prog), shipped, statusInput(cancelled)},
		"move_to": []map[string]any{{"from": review.Id, "to": prog.Id}},
	}
	if code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", body, &list); code != http.StatusOK ||
		!slices.Equal(statusNames(list), []string{"To do", "Blocked", "In progress", "Shipped", "Cancelled"}) {
		t.Fatalf("update: %d %+v", code, list)
	}
	moved, err := e.q.GetTicketByKey(ctx, tk.Key)
	if err != nil || moved.Ticket.StatusID != prog.Id || moved.Ticket.Version != 3 { // seeded 1, In review 2, moved 3
		t.Fatalf("ticket after the move: %+v %v", moved.Ticket, err)
	}
}

func TestStatusRules(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &list)
	rows := func() []map[string]any {
		out := make([]map[string]any, len(list.Items))
		for i, s := range list.Items {
			out[i] = statusInput(s)
		}
		return out
	}
	noDefault, doneDefault, noCancelled, twins, badColor := rows(), rows(), rows()[:4], rows(), rows()
	noDefault[0]["is_default"] = false
	doneDefault[0]["is_default"], doneDefault[3]["is_default"] = false, true
	twins[1]["name"] = "to do"
	badColor[2]["color"] = "blue"
	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{map[string]any{"statuses": noDefault}, "statuses", "invalid"},
		{map[string]any{"statuses": doneDefault}, "statuses[3].is_default", "invalid"},
		{map[string]any{"statuses": noCancelled}, "statuses", "invalid"},
		{map[string]any{"statuses": twins}, "statuses[1].name", "status_name_taken"},
		{map[string]any{"statuses": badColor}, "statuses[2].color", "invalid"},
		{map[string]any{"statuses": append(rows()[:2], rows()[3:]...), // In review goes; its tickets may not land in Done
			"move_to": []map[string]any{{"from": list.Items[2].Id, "to": list.Items[3].Id}}}, "move_to[0]", "invalid"},
	} {
		var prob httpapi.Problem
		code := e.call(admin, http.MethodPut, "/projects/HRIS/statuses", c.body, &prob)
		if f := firstError(prob); code != http.StatusUnprocessableEntity || f.Field != c.field || f.Code != c.code {
			t.Errorf("%s: %d %+v", c.field, code, prob)
		}
	}
}

func TestMembersReadStatusesButOnlyAdminsEditThem(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	viewer, vu := e.signedIn("viewer@example.com", false)
	e.seedMember(vu, p, "viewer")
	var list httpapi.StatusList
	if code := e.call(viewer, http.MethodGet, "/projects/HRIS/statuses", nil, &list); code != http.StatusOK || len(list.Items) != 5 {
		t.Fatalf("viewer reads: %d", code)
	}
	body := map[string]any{"statuses": []map[string]any{statusInput(list.Items[0])}}
	if code := e.call(viewer, http.MethodPut, "/projects/HRIS/statuses", body, nil); code != http.StatusForbidden {
		t.Fatalf("viewer edits: %d", code)
	}
}

func TestProjectKeyIsFixedOnceTheProjectHasTickets(t *testing.T) {
	e := newEnv(t)
	e.seedProject("HRIS")
	admin, au := e.signedIn("admin@example.com", true)
	if code := e.call(admin, http.MethodPatch, "/projects/HRIS", map[string]any{"key": "HR"}, nil); code != http.StatusOK {
		t.Fatalf("rename before tickets: %d", code)
	}
	p, err := e.q.GetProjectByKey(context.Background(), "HR")
	if err != nil {
		t.Fatal(err)
	}
	e.seedTicket(p, au, "First ticket", nil)
	var prob httpapi.Problem
	if code := e.call(admin, http.MethodPatch, "/projects/HR", map[string]any{"key": "HX"}, &prob); code != http.StatusUnprocessableEntity || firstError(prob).Code != "project_key_fixed" {
		t.Fatalf("rename after a ticket: %d %+v", code, prob)
	}
	if code := e.call(admin, http.MethodPatch, "/projects/HR", map[string]any{"key": "hr", "name": "HR System"}, nil); code != http.StatusOK {
		t.Fatalf("same key, new name: %d", code)
	}
}

func TestMembersSeeProjectClientsInTheirScope(t *testing.T) {
	e := newEnv(t)
	a, b := e.seedClient("Client A"), e.seedClient("Client B")
	p := e.seedProject("HRIS", a, b)
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, p, "member", b)
	var list httpapi.ClientList
	if code := e.call(budi, http.MethodGet, "/projects/HRIS/clients", nil, &list); code != http.StatusOK || len(list.Items) != 1 || list.Items[0].Name != "Client B" {
		t.Fatalf("scoped member: %d %+v", code, list)
	}
}

func TestAssigneesAreMembersWhoAreNotViewers(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	e.seedMember(e.seedUser("budi@example.com", pw, false), p, "member")
	e.seedMember(e.seedUser("vera@example.com", pw, false), p, "viewer")
	var list httpapi.RefList
	e.call(owner, http.MethodGet, "/projects/HRIS/assignees", nil, &list)
	var names []string
	for _, u := range list.Items {
		names = append(names, u.Name)
	}
	if !slices.Equal(names, []string{"budi@example.com", "owner@example.com"}) {
		t.Fatalf("assignees: %v", names)
	}
}
