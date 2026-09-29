package docs

import (
	"fmt"
	"testing"
)

const fsd = `# FSD — Muasal

Intro text.

## 7. F1 — Module registry

### 7.4 Overtime Approval

The Behaviors by client tab shows the decisions in force.

### 7.4 Overtime Approvals

` + "```" + `
## not a heading
` + "```" + `

## Ticketing

### Ticket fields
`

func TestSplit(t *testing.T) {
	s := Split(fsd, "FSD")
	got := ""
	for _, x := range s {
		got += fmt.Sprintf("%s|%s|%d;", x.Number, x.Title, x.Level)
	}
	want := "s1|FSD — Muasal|1;7|F1 — Module registry|2;7.4|Overtime Approval|3;7.4-2|Overtime Approvals|3;s5|Ticketing|2;s6|Ticket fields|3;"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	if s[2].Body != "The Behaviors by client tab shows the decisions in force." || s[3].Body == "" {
		t.Fatalf("bodies: %q / %q", s[2].Body, s[3].Body)
	}
}

// R-MR-11: level-2 headings become modules and level-3 headings menus.
func TestFromHeadingsAndMerge(t *testing.T) {
	cands := FromHeadings(Split(fsd, "FSD"))
	nodes := Merge(cands, []Existing{{ID: 1, Path: []string{"Ticketing"}}})
	paths := Paths(nodes)
	got := ""
	for _, n := range nodes {
		got += fmt.Sprintf("%v %s keep=%v exists=%v dup=%q %v;", paths[n.TmpID], n.Type, n.Keep, n.Exists, n.Duplicate, n.Sections)
	}
	want := "[F1 — Module registry] module keep=true exists=false dup=\"\" [7];" +
		"[F1 — Module registry Overtime Approval] menu keep=true exists=false dup=\"\" [7.4];" +
		"[F1 — Module registry Overtime Approvals] menu keep=true exists=false dup=\"n2\" [7.4-2];" +
		"[Ticketing] module keep=false exists=true dup=\"\" [s5];" +
		"[Ticketing Ticket fields] menu keep=true exists=false dup=\"\" [s6];"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// Model paths merge case- and space-insensitively; missing parents become modules.
func TestMergeCandidates(t *testing.T) {
	nodes := Merge([]Candidate{
		{Path: []string{"HR", "Attendance", "Overtime Approval"}, Type: "menu", Aliases: []string{"OT approval", "ot APPROVAL"}, Section: "3.1"},
		{Path: []string{" hr ", "attendance", "overtime  approval"}, Type: "menu", Description: "Approves overtime.", Section: "3.2"},
	}, nil)
	if len(nodes) != 3 || nodes[0].Name != "HR" || nodes[0].Type != "module" || nodes[1].Type != "module" {
		t.Fatalf("%+v", nodes)
	}
	ot := nodes[2]
	if fmt.Sprintf("%v %v %s", ot.Aliases, ot.Sections, ot.Description) != "[OT approval] [3.1 3.2] Approves overtime." {
		t.Fatalf("%+v", ot)
	}
}

func TestSimilarity(t *testing.T) {
	if s := Similarity("Overtime Approval", "Overtime Approvals"); s < 0.8 {
		t.Fatalf("near names: %v", s)
	}
	if s := Similarity("Overtime Approval", "Leave Balance"); s > 0.2 {
		t.Fatalf("different names: %v", s)
	}
}

// MSL-3, MSL-17: a heading's trailing ID in brackets is split off; words in
// brackets are part of the name.
func TestHeadingName(t *testing.T) {
	for title, want := range map[string][2]string{
		"Persetujuan Sales Order (SO-02)": {"Persetujuan Sales Order", "SO-02"},
		"Laporan Umur Piutang (AR-03) ":   {"Laporan Umur Piutang", "AR-03"},
		"Catatan (Opsional)":              {"Catatan (Opsional)", ""},
		"Overtime Approval":               {"Overtime Approval", ""},
	} {
		if name, id := HeadingName(title); [2]string{name, id} != want {
			t.Errorf("%q: %q %q, want %v", title, name, id, want)
		}
	}
}
