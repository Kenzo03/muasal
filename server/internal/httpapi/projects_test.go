package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

func TestAdminCreatesAProjectAndMembersSeeOnlyTheirProjects(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	var p httpapi.Project
	code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": " hris ", "name": " HRIS ", "description": "Human resources"}, &p)
	if code != http.StatusCreated || p.Key != "HRIS" || p.Name != "HRIS" || p.Role != httpapi.ProjectRoleAdmin {
		t.Fatalf("create: %d %+v", code, p)
	}
	// MSL-47: the creator is the new project's admin, so they can be assigned.
	var members httpapi.MemberList
	if e.call(admin, http.MethodGet, "/projects/HRIS/members", nil, &members); len(members.Items) != 1 ||
		members.Items[0].Email != "admin@example.com" || members.Items[0].Role != httpapi.ProjectRoleAdmin {
		t.Fatalf("members after create: %+v", members.Items)
	}
	pay := e.seedProject("PAY")
	budi, bu := e.signedIn("budi@example.com", false)
	e.seedMember(bu, pay, "viewer")
	var list httpapi.ProjectList
	if code := e.call(budi, http.MethodGet, "/projects", nil, &list); code != http.StatusOK ||
		len(list.Items) != 1 || list.Items[0].Key != "PAY" || list.Items[0].Role != httpapi.ProjectRoleViewer {
		t.Fatalf("budi's projects: %d %+v", code, list)
	}
	if code := e.call(budi, http.MethodGet, "/projects/HRIS", nil, nil); code != http.StatusNotFound {
		t.Fatalf("a non-member must get 404, got %d", code)
	}
	if code := e.call(admin, http.MethodGet, "/projects", nil, &list); code != http.StatusOK || len(list.Items) != 2 {
		t.Fatalf("a system admin sees every project: %d %+v", code, list)
	}
	if code := e.call(budi, http.MethodPost, "/projects", map[string]any{"key": "NEW", "name": "New"}, nil); code != http.StatusForbidden {
		t.Fatalf("only system admins create projects, got %d", code)
	}
}

func TestProjectKeyRules(t *testing.T) {
	e := newEnv(t)
	admin, _ := e.signedIn("admin@example.com", true)
	for _, key := range []string{"H", "1HR", "HR-IS", "ABCDEFGHIJK"} {
		var p httpapi.Problem
		if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": key, "name": "X"}, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "key" {
			t.Errorf("key %q: %d %+v", key, code, p)
		}
	}
	e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "HRIS"}, nil)
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/projects", map[string]any{"key": "HRIS", "name": "Again"}, &p); code != http.StatusConflict || p.Code != "project_key_taken" {
		t.Fatalf("duplicate key: %d %+v", code, p)
	}
}

func TestOnlyProjectAdminsEditAProjectAndEditsAreAudited(t *testing.T) {
	e := newEnv(t)
	p := e.seedProject("HRIS")
	member, mu := e.signedIn("member@example.com", false)
	e.seedMember(mu, p, "member")
	owner, ou := e.signedIn("owner@example.com", false)
	e.seedMember(ou, p, "admin")
	var prob httpapi.Problem
	if code := e.call(member, http.MethodPatch, "/projects/HRIS", map[string]any{"name": "Nope"}, &prob); code != http.StatusForbidden || prob.Code != "forbidden" {
		t.Fatalf("member edit: %d %+v", code, prob)
	}
	var out httpapi.Project
	if code := e.call(owner, http.MethodPatch, "/projects/HRIS", map[string]any{"name": "HR System", "key": "hrs"}, &out); code != http.StatusOK ||
		out.Key != "HRS" || out.Name != "HR System" || out.Role != httpapi.ProjectRoleAdmin {
		t.Fatalf("admin edit: %d %+v", code, out)
	}
	events, err := e.q.ListAuditEvents(context.Background(), db.ListAuditEventsParams{Entity: "project", EntityID: p.ID})
	if err != nil || len(events) != 1 || events[0].ProjectID == nil || *events[0].ProjectID != p.ID {
		t.Fatalf("audit: %+v %v", events, err)
	}
	var changes map[string]map[string]any
	if err := json.Unmarshal(events[0].Changes, &changes); err != nil ||
		changes["name"]["old"] != "HRIS" || changes["name"]["new"] != "HR System" || changes["description"] != nil {
		t.Fatalf("changes: %s", events[0].Changes)
	}
}
