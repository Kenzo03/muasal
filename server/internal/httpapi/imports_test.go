package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/httpapi"
	"github.com/kenzo03/zettra/server/internal/ticketimport"
)

const jiraFile = `Summary,Issue key,Issue Type,Status,Reporter,Created,Resolved,Component/s,Comment
Overtime export for payroll,PAY-332,Story,Done,Budi,12/Mar/24 2:05 PM,20/Mar/24 9:00 AM,Overtime Approval,14/Mar/24 10:00 AM;Budi;Format confirmed.
Spike without a type,PAY-333,Spike,To Do,Budi,13/Mar/24 9:00 AM,,,
`

func (e *env) upload(c *http.Client, fields map[string]string, file string) (int, httpapi.ImportRun) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", "jira.csv")
	fw.Write([]byte(file))
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/imports", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out httpapi.ImportRun
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// §14.2: a system admin uploads a Jira export, reads the dry run, runs it;
// the old key then finds the ticket (R-IN-2).
func TestJiraImportThroughTheAPI(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	if code, _ := e.upload(w.pm, map[string]string{"project_key": "HRIS", "preset": "jira"}, jiraFile); code != http.StatusForbidden {
		t.Fatalf("a member imports: %d", code)
	}
	code, run := e.upload(admin, map[string]string{"project_key": "hris", "preset": "jira"}, jiraFile)
	if code != http.StatusCreated || run.Status != httpapi.ImportRunStatusDryRun || run.Stats.Create != 1 || run.Stats.Reject != 1 ||
		run.Stats.Coverage != 100 || len(run.Errors) != 1 || run.Errors[0].Line != 3 || len(run.Headers) != 9 {
		t.Fatalf("dry run: %d %+v", code, run)
	}
	// Mapping Spike to a type fixes the rejected row.
	m := run.Mapping
	types := map[string]string{"story": "feature", "spike": "change_request"}
	m.Types = &types
	var planned httpapi.ImportRun
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/imports/%d/plan", run.Id), m, &planned); code != http.StatusOK || planned.Stats.Reject != 0 || planned.Stats.Create != 2 {
		t.Fatalf("plan: %d %+v", code, planned.Stats)
	}
	var started httpapi.ImportRun
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/imports/%d/run", run.Id), nil, &started); code != http.StatusAccepted || started.Status != httpapi.ImportRunStatusRunning {
		t.Fatalf("run: %d %+v", code, started)
	}
	if code := e.call(admin, http.MethodPost, fmt.Sprintf("/imports/%d/run", run.Id), nil, nil); code != http.StatusConflict {
		t.Fatalf("a second run: %d", code)
	}
	// The worker's part, as `app serve` would run it.
	if err := ticketimport.Run(context.Background(), e.d.Pool, run.Id, func(context.Context, pgx.Tx, []int64) error { return nil }); err != nil {
		t.Fatal(err)
	}
	var done httpapi.ImportRun
	if e.call(admin, http.MethodGet, fmt.Sprintf("/imports/%d", run.Id), nil, &done); done.Status != httpapi.ImportRunStatusDone || done.Stats.Tickets != 2 || done.Stats.Comments != 1 {
		t.Fatalf("done: %+v", done)
	}
	var found httpapi.SearchResults
	if e.call(admin, http.MethodGet, "/search?q=pay-332", nil, &found); len(found.Tickets) != 1 || found.Tickets[0].Title != "Overtime export for payroll" {
		t.Fatalf("old key search: %+v", found.Tickets)
	}
	var list httpapi.ImportRunList
	if e.call(admin, http.MethodGet, "/imports", nil, &list); len(list.Items) != 1 {
		t.Fatalf("list: %+v", list)
	}
}

// MSL-49: a project admin imports into their project without a system admin;
// the import stays out of sight for members and other projects' admins.
func TestProjectAdminsImportTheirBacklog(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, lu := e.signedIn("lead@example.com", false)
	e.seedMember(lu, w.p, "admin")
	code, run := e.upload(lead, map[string]string{"project_key": "HRIS", "preset": "jira"}, jiraFile)
	if code != http.StatusCreated || run.Status != httpapi.ImportRunStatusDryRun {
		t.Fatalf("a project admin's dry run: %d %+v", code, run)
	}
	other, ou := e.signedIn("other@example.com", false)
	e.seedMember(ou, e.seedProject("PAY"), "admin")
	for who, c := range map[string]*http.Client{"a member": w.pm, "another project's admin": other} {
		var list httpapi.ImportRunList
		if e.call(c, http.MethodGet, "/imports", nil, &list); len(list.Items) != 0 {
			t.Errorf("%s sees the import: %+v", who, list.Items)
		}
		if code := e.call(c, http.MethodGet, fmt.Sprintf("/imports/%d", run.Id), nil, nil); code != http.StatusNotFound {
			t.Errorf("%s opens the import: %d", who, code)
		}
	}
	if code, _ := e.upload(other, map[string]string{"project_key": "HRIS", "preset": "jira"}, jiraFile); code != http.StatusForbidden {
		t.Fatalf("another project's admin imports into HRIS: %d", code)
	}
	var started httpapi.ImportRun
	if code := e.call(lead, http.MethodPost, fmt.Sprintf("/imports/%d/run", run.Id), nil, &started); code != http.StatusAccepted {
		t.Fatalf("the project admin runs it: %d", code)
	}
}
