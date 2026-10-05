package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/draft"
	"github.com/kenzo03/zettra/server/internal/httpapi"
	"github.com/kenzo03/zettra/server/internal/indexer"
	"github.com/kenzo03/zettra/server/internal/llm/llmtest"
)

func (e *env) uploadDoc(c *http.Client, fields map[string]string, name, content string) (int, httpapi.Document) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write([]byte(content))
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, e.url+"/api/v1/projects/HRIS/documents", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	var out httpapi.Document
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func (e *env) nodeCount() int {
	e.t.Helper()
	var n int
	if err := e.d.Pool.QueryRow(context.Background(), "SELECT count(*) FROM nodes").Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

const hrisFSD = `# HRIS FSD

## HR

### Overtime Approval

Supervisors approve overtime before payroll.

### Leave Balance (HR-LV-01)

Shows the leave each employee has left.

## Payroll

### Payslip

The monthly payslip.
`

// §7.7 with AI off (R-MR-11, AC-MR-10): headings propose the tree at once,
// without a model; the admin reviews, and applying creates only the ticked
// nodes the tree lacks, parents first, linked to their sections.
func TestDocumentTreeFromHeadings(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	bayu, bayuUser := e.signedIn("bayu@example.com", false)
	e.seedMember(bayuUser, w.p, "member", w.b)

	fields := map[string]string{"title": "HRIS FSD", "markdown": hrisFSD, "client_id": strconv.FormatInt(w.a.ID, 10)}
	if code, _ := e.uploadDoc(w.pm, fields, "fsd.md", hrisFSD); code != http.StatusForbidden {
		t.Fatalf("a member uploads: %d", code)
	}
	if code, _ := e.uploadDoc(lead, fields, "fsd.exe", hrisFSD); code != http.StatusUnsupportedMediaType {
		t.Fatalf("an exe: %d", code)
	}
	code, doc := e.uploadDoc(lead, fields, "fsd.md", hrisFSD)
	if code != http.StatusCreated || doc.Key != "HRIS-DOC1" || len(doc.Sections) != 6 || doc.Sections[2].Title != "Overtime Approval" {
		t.Fatalf("upload: %d %+v", code, doc)
	}
	if code := e.call(bayu, http.MethodGet, "/documents/HRIS-DOC1", nil, nil); code != http.StatusNotFound {
		t.Fatalf("a Client B member reads a Client A document: %d", code)
	}
	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/documents/hris-doc1/file", nil)
	res, err := w.pm.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	file, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(file) != hrisFSD || !strings.Contains(res.Header.Get("Content-Disposition"), "fsd.md") {
		t.Fatalf("download: %q %q", file, res.Header.Get("Content-Disposition"))
	}

	before := e.nodeCount()
	if code := e.call(w.pm, http.MethodPost, "/documents/HRIS-DOC1/tree-drafts", nil, nil); code != http.StatusForbidden {
		t.Fatalf("a member drafts: %d", code)
	}
	var d httpapi.TreeDraft
	if code := e.call(lead, http.MethodPost, "/documents/HRIS-DOC1/tree-drafts", nil, &d); code != http.StatusAccepted || d.Status != "ready" || d.UsedAi {
		t.Fatalf("draft: %d %+v", code, d)
	}
	got := map[string]httpapi.TreeDraftNode{}
	for _, n := range d.Proposal {
		got[n.Name] = n
		if len(n.Sections) == 0 {
			t.Fatalf("a node without its source section: %+v", n) // AC-MR-8
		}
	}
	if len(d.Proposal) != 5 || !got["HR"].Exists || got["HR"].Keep || !got["Overtime Approval"].Exists || !got["Leave Balance"].Keep || !got["Payslip"].Keep {
		t.Fatalf("proposal: %+v", d.Proposal)
	}
	if e.nodeCount() != before {
		t.Fatal("drafting changed the tree")
	}

	// The admin renames one node and unticks Payroll but not its Payslip.
	for i, n := range d.Proposal {
		switch n.Name {
		case "Leave Balance":
			d.Proposal[i].Name = "Leave Balances"
		case "Payroll":
			d.Proposal[i].Keep = false
		}
	}
	path := "/tree-drafts/" + strconv.FormatInt(d.Id, 10)
	if code := e.call(lead, http.MethodPut, path, map[string]any{"proposal": d.Proposal}, &d); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	var p httpapi.Problem
	if code := e.call(lead, http.MethodPost, path+"/apply", nil, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "parent_unticked" {
		t.Fatalf("an unticked parent: %d %+v", code, p)
	}
	for i, n := range d.Proposal {
		if n.Name == "Payslip" {
			d.Proposal[i].Keep = false
		}
	}
	e.call(lead, http.MethodPut, path, map[string]any{"proposal": d.Proposal}, nil)
	var applied struct{ Created, Linked int }
	if code := e.call(lead, http.MethodPost, path+"/apply", nil, &applied); code != http.StatusOK || applied.Created != 1 || applied.Linked != 3 {
		t.Fatalf("apply: %d %+v", code, applied)
	}
	var source, parent string
	var nodeCode *string
	if err := e.d.Pool.QueryRow(t.Context(), `SELECT n.source, p.name, n.code FROM nodes n JOIN nodes p ON p.id = n.parent_id WHERE n.name = 'Leave Balances'`).
		Scan(&source, &parent, &nodeCode); err != nil || source != "ai_draft" || parent != "HR" || nodeCode == nil || *nodeCode != "HR-LV-01" {
		t.Fatalf("created node: %v %q %q %v", err, source, parent, nodeCode) // MSL-17: the heading's ID is its code
	}
	if code := e.call(lead, http.MethodPost, path+"/apply", nil, nil); code != http.StatusConflict {
		t.Fatalf("applied twice: %d", code)
	}
	e.call(lead, http.MethodGet, "/documents/HRIS-DOC1", nil, &doc)
	if doc.Sections[2].Title != "Overtime Approval" || len(doc.Sections[2].Nodes) != 1 || doc.Sections[2].Nodes[0].Id != w.ot.ID ||
		len(doc.Drafts) != 1 || doc.Drafts[0].Status != "applied" {
		t.Fatalf("document after apply: %+v", doc)
	}

	// A node's timeline starts from the sections it came from, with its
	// sub-nodes' by default; a Client B member never sees a Client A document.
	sections := func(c *http.Client, path string) []string {
		t.Helper()
		var tl httpapi.TimelinePage
		if code := e.call(c, http.MethodGet, path, nil, &tl); code != http.StatusOK {
			t.Fatalf("%s: %d", path, code)
		}
		out := []string{}
		for _, s := range tl.Sections {
			out = append(out, s.Key+" "+s.Title)
		}
		return out
	}
	hr := fmt.Sprintf("/nodes/%d/timeline", w.hr.ID)
	if got := sections(lead, fmt.Sprintf("/nodes/%d/timeline", w.ot.ID)); len(got) != 1 || got[0] != "HRIS-DOC1/"+doc.Sections[2].Number+" Overtime Approval" {
		t.Fatalf("Overtime Approval's sections: %v", got)
	}
	if got := sections(lead, hr); len(got) != 3 {
		t.Fatalf("HR with its sub-nodes: %v", got)
	}
	if got := sections(lead, hr+"?sub_nodes=false"); len(got) != 1 {
		t.Fatalf("HR alone: %v", got)
	}
	if got := sections(bayu, hr); len(got) != 0 {
		t.Fatalf("a Client B member sees a Client A document: %v", got)
	}
}

// §7.7: an admin marks a document replaced after its upload, when the upload
// did not say so, or current again. Only a current document can replace one,
// so replacements never loop.
func TestDocumentReplacedAfterUpload(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	fields := map[string]string{"title": "HRIS FSD", "markdown": hrisFSD}
	e.uploadDoc(lead, fields, "fsd-v1.md", hrisFSD)
	if code, doc := e.uploadDoc(lead, fields, "fsd-v2.md", hrisFSD); code != http.StatusCreated || doc.Key != "HRIS-DOC2" {
		t.Fatalf("second upload: %d %+v", code, doc)
	}
	replace := func(c *http.Client, key string, by any) (int, *string) {
		t.Helper()
		var doc httpapi.Document
		code := e.call(c, http.MethodPatch, "/documents/"+key, map[string]any{"superseded_by": by}, &doc)
		return code, doc.SupersededBy
	}
	if code, _ := replace(w.pm, "HRIS-DOC1", "HRIS-DOC2"); code != http.StatusForbidden {
		t.Fatalf("a member replaces a document: %d", code)
	}
	for _, by := range []string{"HRIS-DOC1", "HRIS-DOC9", "OTHER-DOC1"} {
		if code, _ := replace(lead, "HRIS-DOC1", by); code != http.StatusUnprocessableEntity {
			t.Fatalf("replaced by %s: %d", by, code)
		}
	}
	if code, by := replace(lead, "hris-doc1", "hris-doc2"); code != http.StatusOK || by == nil || *by != "HRIS-DOC2" {
		t.Fatalf("replace: %d %v", code, by)
	}
	if code, _ := replace(lead, "HRIS-DOC2", "HRIS-DOC1"); code != http.StatusUnprocessableEntity {
		t.Fatalf("a replaced document replaces its successor: %d", code)
	}
	if code, by := replace(lead, "HRIS-DOC1", nil); code != http.StatusOK || by != nil {
		t.Fatalf("current again: %d %v", code, by)
	}
	var n int
	if err := e.d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events WHERE entity = 'document' AND action = 'update'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("audit events: %v %d", err, n)
	}
}

// §7.7 with AI on (AC-MR-8, AC-MR-9): the model proposes nodes with their
// source sections in a background job; after applying, Ask cites the section.
func TestDocumentTreeFromTheModelAndAskCitesIt(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	fake := e.localAI(admin)
	fake.Set(func(s *llmtest.Server) {
		s.Answer = func(system, user string, schema json.RawMessage) string {
			if strings.Contains(system, "functional specification") {
				return `{"nodes":[{"path":["Registry","Node page"],"type":"menu","aliases":["Menu page"],"description":"One menu's history.","section":"7.4"}]}`
			}
			var sc struct {
				Properties struct {
					Claims struct {
						Items struct {
							Properties struct {
								Cites struct {
									Items struct {
										Enum []string `json:"enum"`
									} `json:"items"`
								} `json:"cites"`
							} `json:"properties"`
						} `json:"items"`
					} `json:"claims"`
				} `json:"properties"`
			}
			_ = json.Unmarshal(schema, &sc)
			for _, k := range sc.Properties.Claims.Items.Properties.Cites.Items.Enum {
				if strings.HasSuffix(k, "/7.4") {
					return fmt.Sprintf(`{"claims":[{"text":"The tab shows the decisions in force for each client.","cites":[%q]}]}`, k)
				}
			}
			return `{"claims":[]}`
		}
	})
	md := "# Zettra FSD\n\n## 7. Module registry\n\nThe tree of modules.\n\n### 7.4 Node page\n\nThe Behaviors by client tab shows the decisions in force for each client.\n"
	code, doc := e.uploadDoc(lead, map[string]string{"title": "Zettra FSD", "markdown": md}, "fsd.docx", "PK-docx-bytes")
	if code != http.StatusCreated {
		t.Fatalf("upload: %d", code)
	}
	before := e.nodeCount()
	var d httpapi.TreeDraft
	if code := e.call(lead, http.MethodPost, "/documents/"+doc.Key+"/tree-drafts", nil, &d); code != http.StatusAccepted || d.Status != "running" || !d.UsedAi || d.TotalParts != 1 {
		t.Fatalf("start: %d %+v", code, d)
	}
	if err := draft.RunTreeDraft(t.Context(), e.d.Pool, e.api.AI(), d.Id); err != nil { // as the worker would
		t.Fatal(err)
	}
	path := "/tree-drafts/" + strconv.FormatInt(d.Id, 10)
	e.call(lead, http.MethodGet, path, nil, &d)
	if d.Status != "ready" || d.DoneParts != 1 || len(d.Proposal) != 2 || d.Proposal[0].Name != "Registry" || d.Proposal[1].Sections[0] != "7.4" ||
		d.Proposal[1].Aliases[0] != "Menu page" || e.nodeCount() != before {
		t.Fatalf("ready: %+v", d)
	}
	if got := notes(e, lead); len(got.Items) != 1 || got.Items[0].Type != httpapi.NotificationTypeJobDone {
		t.Fatalf("the starter was not told: %+v", got)
	}
	var applied struct{ Created, Linked int }
	if code := e.call(lead, http.MethodPost, path+"/apply", nil, &applied); code != http.StatusOK || applied.Created != 2 || applied.Linked != 1 {
		t.Fatalf("apply: %d %+v", code, applied)
	}

	rows, _ := e.d.Pool.Query(t.Context(), "SELECT id FROM document_sections ORDER BY id")
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	ix := indexer.New(e.d.Pool, e.api.AI())
	for _, id := range ids {
		if err := ix.RebuildSection(t.Context(), id); err != nil {
			t.Fatal(err)
		}
		if err := ix.EmbedSection(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	events := e.askStream(lead, map[string]any{"question": "What does the Behaviors by client tab show?"})
	var claim httpapi.AskClaim
	for _, ev := range events {
		if ev.name == "claim" {
			_ = json.Unmarshal(ev.data, &claim)
		}
	}
	if len(claim.Cites) != 1 || claim.Cites[0] != doc.Key+"/7.4" {
		t.Fatalf("Ask did not cite the section: %+v %v", claim, events)
	}
}

// MSL-15: search finds words that only a document's text holds, with an
// excerpt, and never shows a member another client's document.
func TestSearchFindsDocumentText(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e) // the PM sees Client A only
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	e.uploadDoc(lead, map[string]string{"title": "HRIS FSD", "markdown": hrisFSD, "client_id": strconv.FormatInt(w.a.ID, 10)}, "fsd.md", hrisFSD)
	var found httpapi.SearchResults
	if code := e.call(w.pm, http.MethodGet, "/search?q=leave+each+employee", nil, &found); code != http.StatusOK || len(found.Sections) != 1 ||
		found.Sections[0].DocumentKey != "HRIS-DOC1" || !strings.Contains(found.Sections[0].Excerpt, "leave each employee") {
		t.Fatalf("search: %d %+v", code, found.Sections)
	}
	e.uploadDoc(lead, map[string]string{"title": "Client B manual", "markdown": "# Manual\n\n## Exports\n\nThe zebra export runs nightly.", "client_id": strconv.FormatInt(w.b.ID, 10)}, "b.md",
		"# Manual\n\n## Exports\n\nThe zebra export runs nightly.")
	if e.call(w.pm, http.MethodGet, "/search?q=zebra", nil, &found); len(found.Sections) != 0 {
		t.Fatalf("a Client A member sees Client B's document: %+v", found.Sections)
	}
	if e.call(lead, http.MethodGet, "/search?q=zebra", nil, &found); len(found.Sections) != 1 {
		t.Fatalf("the lead: %+v", found.Sections)
	}
}

// MSL-14: a document that replaces another keeps its menus, section by
// section, and says what it replaced; marking one replaced later does too.
func TestReplacingADocumentKeepsItsMenus(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	lead, leadUser := e.signedIn("lead@example.com", false)
	e.seedMember(leadUser, w.p, "admin")
	fields := map[string]string{"title": "HRIS FSD", "markdown": hrisFSD}
	e.uploadDoc(lead, fields, "fsd-v1.md", hrisFSD)
	if _, err := e.d.Pool.Exec(t.Context(), `INSERT INTO document_section_nodes (section_id, node_id)
		SELECT s.id, $1 FROM document_sections s JOIN documents d ON d.id = s.document_id WHERE d.key = 'HRIS-DOC1' AND s.title = 'Overtime Approval'`, w.ot.ID); err != nil {
		t.Fatal(err)
	}
	menusOf := func(doc httpapi.Document) []int64 {
		for _, sec := range doc.Sections {
			if sec.Title == "Overtime Approval" {
				ids := []int64{}
				for _, n := range sec.Nodes {
					ids = append(ids, n.Id)
				}
				return ids
			}
		}
		return nil
	}
	v2 := strings.Replace(hrisFSD, "Supervisors approve", "HR approves", 1)
	code, doc := e.uploadDoc(lead, map[string]string{"title": "HRIS FSD v2", "markdown": v2, "supersedes": "HRIS-DOC1"}, "fsd-v2.md", v2)
	if code != http.StatusCreated || !slices.Equal(menusOf(doc), []int64{w.ot.ID}) || len(doc.Replaces) != 1 || doc.Replaces[0].Key != "HRIS-DOC1" {
		t.Fatalf("upload replacing: %d %v %+v", code, menusOf(doc), doc.Replaces)
	}
	e.uploadDoc(lead, map[string]string{"title": "HRIS FSD v3", "markdown": hrisFSD}, "fsd-v3.md", hrisFSD)
	e.call(lead, http.MethodPatch, "/documents/HRIS-DOC2", map[string]any{"superseded_by": "HRIS-DOC3"}, nil)
	e.call(lead, http.MethodGet, "/documents/HRIS-DOC3", nil, &doc)
	if !slices.Equal(menusOf(doc), []int64{w.ot.ID}) || len(doc.Replaces) != 1 || doc.Replaces[0].Key != "HRIS-DOC2" {
		t.Fatalf("replaced later: %v %+v", menusOf(doc), doc.Replaces)
	}
	// MSL-39: Behaviours' baseline is the section of the document in force.
	var b httpapi.BehaviorList
	if code := e.call(lead, http.MethodGet, fmt.Sprintf("/nodes/%d/behaviors", w.ot.ID), nil, &b); code != http.StatusOK ||
		len(b.Sections) != 1 || b.Sections[0].DocumentKey != "HRIS-DOC3" || b.Sections[0].Title != "Overtime Approval" {
		t.Fatalf("behaviours baseline: %d %+v", code, b.Sections)
	}
}
