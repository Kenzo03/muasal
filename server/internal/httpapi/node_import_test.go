package httpapi_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

func (e *env) importTree(c *http.Client, key, csv string, dryRun bool) (int, httpapi.NodeImportResult) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "tree.csv")
	fw.Write([]byte(csv))
	if dryRun {
		mw.WriteField("dry_run", "true")
	}
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/projects/"+key+"/nodes/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out httpapi.NodeImportResult
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

const treeCSV = `path,type,code,client_scope,clients,aliases
HR,module,,shared,,
HR > Attendance,module,HR.ATT,shared,,
HR > Attendance > Overtime Approval,menu,HR.ATT.OT,client_specific,Client A,Persetujuan Lembur;OT approval
HR > Attendance > Clock In,menu,HR.ATT.CI,shared,,
`

// §7.5: a dry run plans, the import applies, and a re-import changes nothing.
// HR and Overtime Approval exist already, matched by path; Attendance is new.
func TestTreeImportPlansThenApplies(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	if code, _ := e.importTree(w.pm, "HRIS", treeCSV, true); code != http.StatusForbidden {
		t.Fatalf("a member imports: %d", code)
	}
	code, plan := e.importTree(admin, "HRIS", treeCSV, true)
	if code != http.StatusOK || plan.Applied || len(plan.Problems) != 0 || len(plan.Created) != 3 || plan.Unchanged != 1 ||
		!slices.Contains(plan.Missing, "HR › Overtime Approval") || !slices.Contains(plan.Missing, "HR › Client B Report") {
		t.Fatalf("dry run: %d %+v", code, plan)
	}
	var nodes httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &nodes)
	before := len(nodes.Items)

	code, done := e.importTree(admin, "HRIS", treeCSV, false)
	if code != http.StatusOK || !done.Applied {
		t.Fatalf("apply: %d %+v", code, done)
	}
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &nodes)
	if len(nodes.Items) != before+3 {
		t.Fatalf("%d nodes, had %d", len(nodes.Items), before)
	}
	for _, n := range nodes.Items {
		if n.Name == "Overtime Approval" && n.Code != nil && *n.Code == "HR.ATT.OT" {
			if !n.ClientSpecific || len(n.Clients) != 1 || n.Clients[0].Name != "Client A" || !slices.Contains(n.Aliases, "OT approval") {
				t.Fatalf("imported menu: %+v", n)
			}
		}
	}
	if _, again := e.importTree(admin, "HRIS", treeCSV, true); len(again.Created) != 0 || len(again.Changed) != 0 || again.Unchanged != 4 {
		t.Fatalf("re-import: %+v", again)
	}
}

// AC-MR-7: a duplicate sibling name is shown, and nothing imports until it is fixed.
func TestTreeImportStopsOnRowErrors(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	bad := treeCSV + "HR > Attendance > overtime approval,menu,,shared,,\n"
	code, out := e.importTree(admin, "HRIS", bad, false)
	if code != http.StatusOK || out.Applied || len(out.Problems) != 1 || out.Problems[0].Line != 6 || out.Problems[0].Code != httpapi.NodeImportProblemCodeDuplicateName {
		t.Fatalf("import: %d %+v", code, out)
	}
	var nodes httpapi.NodeList
	e.call(admin, http.MethodGet, "/projects/HRIS/nodes", nil, &nodes)
	for _, n := range nodes.Items {
		if n.Name == "Attendance" {
			t.Fatalf("a row imported despite the error")
		}
	}
	_ = w
}
