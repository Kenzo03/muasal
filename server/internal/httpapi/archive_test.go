package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// MSL-64: a project admin archives a finished project. It leaves the project
// list and Home but stays readable under All projects, takes no changes until
// restored, and only its admins archive or restore it.
func TestArchivedProjectsAreReadOnly(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	body := map[string]any{"client_id": w.a.ID, "title": "Payslip layout", "node_ids": []int64{w.ot.ID}, "type": "change_request", "assignee_id": w.pmUser.ID}
	var tk httpapi.Ticket
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated {
		t.Fatalf("ticket: %d", code)
	}

	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/archive", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member archives: %d", code)
	}
	var p httpapi.Project
	if code := e.call(lead, http.MethodPost, "/projects/HRIS/archive", nil, &p); code != http.StatusOK || p.ArchivedAt == nil || p.Role != "viewer" || p.CanRestore == nil || !*p.CanRestore {
		t.Fatalf("archive: %d %+v", code, p)
	}

	var list httpapi.ProjectList
	if e.call(w.pm, http.MethodGet, "/projects", nil, &list); len(list.Items) != 0 {
		t.Fatalf("the picker lists it: %+v", list.Items)
	}
	if e.call(w.pm, http.MethodGet, "/projects?archived=true", nil, &list); len(list.Items) != 1 || list.Items[0].ArchivedAt == nil || *list.Items[0].CanRestore {
		t.Fatalf("All projects: %+v", list.Items)
	}
	var mine httpapi.MyTicketsPage
	if e.call(w.pm, http.MethodGet, "/me/tickets", nil, &mine); len(mine.Items) != 0 {
		t.Fatalf("Home lists it: %+v", mine.Items)
	}
	if code := e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, nil); code != http.StatusOK {
		t.Fatalf("read: %d", code)
	}

	var prob httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &prob); code != http.StatusConflict || prob.Code != "project_archived" {
		t.Fatalf("new ticket: %d %+v", code, prob)
	}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Still here?"}, &prob); code == http.StatusCreated {
		t.Fatalf("comment: %d", code)
	}
	if code := e.call(lead, http.MethodPatch, "/projects/HRIS", map[string]any{"name": "HRIS v2"}, &prob); code != http.StatusConflict {
		t.Fatalf("settings: %d %+v", code, prob)
	}

	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/restore", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member restores: %d", code)
	}
	var back httpapi.Project
	if code := e.call(lead, http.MethodPost, "/projects/HRIS/restore", nil, &back); code != http.StatusOK || back.ArchivedAt != nil || back.Role != "admin" {
		t.Fatalf("restore: %d %+v", code, back)
	}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated {
		t.Fatalf("after restore: %d", code)
	}
	if e.call(w.pm, http.MethodGet, "/me/tickets", nil, &mine); len(mine.Items) != 2 {
		t.Fatalf("Home after restore: %+v", mine.Items)
	}
}
