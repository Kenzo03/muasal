package treeimport

import (
	"fmt"
	"strings"
	"testing"
)

const pathCSV = `path,type,code,client_scope,clients,aliases
HR,module,HR,shared,,
HR > Attendance,module,HR.ATT,shared,,
HR > Attendance > Overtime Approval,menu,HR.ATT.OT,client_specific,Client A;Client C,Persetujuan Lembur;OT approval
`

// §7.5: the path shape, with its client scope, clients and aliases.
func TestParsePathShape(t *testing.T) {
	rows, problems, err := Parse(strings.NewReader(pathCSV))
	if err != nil || len(problems) != 0 || len(rows) != 3 {
		t.Fatalf("%v %v %d", err, problems, len(rows))
	}
	ot := rows[2]
	if Show(ot.Path) != "HR › Attendance › Overtime Approval" || ot.Type != "menu" || ot.Code != "HR.ATT.OT" || !ot.ClientSpecific ||
		fmt.Sprint(ot.Clients) != "[Client A Client C]" || fmt.Sprint(ot.Aliases) != "[Persetujuan Lembur OT approval]" || ot.Line != 4 {
		t.Fatalf("row: %+v", ot)
	}
}

// §7.5: the adjacency shape from the customer's menu table, semicolons too;
// a missing parent and a loop are row errors.
func TestParseAdjacencyShape(t *testing.T) {
	rows, problems, _ := Parse(strings.NewReader("\ufeffid;parent_id;name;type;code\n1;;HR;module;HR\n2;1;Leave;menu;\n3;9;Orphan;menu;\n4;5;Loop A;menu;\n5;4;Loop B;menu;\n"))
	if len(rows) != 2 || Show(rows[1].Path) != "HR › Leave" {
		t.Fatalf("rows: %+v", rows)
	}
	var codes []string
	for _, p := range problems {
		codes = append(codes, fmt.Sprintf("%d:%s", p.Line, p.Code))
	}
	if fmt.Sprint(codes) != "[4:missing_parent 5:cycle 6:cycle]" {
		t.Fatalf("problems: %v", codes)
	}
}

// AC-MR-7: a duplicate sibling name is a row error naming the parent.
func TestDiffFlagsDuplicateSiblings(t *testing.T) {
	rows, _, _ := Parse(strings.NewReader(pathCSV + "HR > Attendance > overtime approval,menu,,shared,,\n"))
	p := Diff(rows, nil, []string{"Client A", "Client C"})
	if len(p.Problems) != 1 || p.Problems[0].Line != 5 || p.Problems[0].Code != "duplicate_name" ||
		!strings.HasPrefix(p.Problems[0].Message, "Duplicate name under HR › Attendance") {
		t.Fatalf("problems: %+v", p.Problems)
	}
}

// Rows match by code, else by path; nothing is deleted, and nodes the file
// leaves out are listed.
func TestDiffMatchesByCodeThenPath(t *testing.T) {
	rows, _, _ := Parse(strings.NewReader(pathCSV + "HR > Leave,menu,,shared,,\nHR > Payroll > Tax,menu,,shared,,\nHR > Attendance > Clock In,menu,,client_specific,Client Z,\n"))
	existing := []Existing{
		{ID: 1, Path: []string{"HR"}, Type: "module", Code: "HR"},
		{ID: 2, Path: []string{"HR", "Time"}, Type: "module", Code: "HR.ATT"}, // renamed in the file
		{ID: 3, Path: []string{"hr", "leave"}, Type: "menu"},                  // matched by path
		{ID: 4, Path: []string{"HR", "Reports"}, Type: "module"},              // not in the file
		{ID: 5, Path: []string{"HR", "Old"}, Type: "menu", Archived: true},    // archived: never listed
	}
	p := Diff(rows, existing, []string{"Client A", "Client C"})
	if len(p.Create) != 1 || Show(p.Create[0].Path) != "HR › Attendance › Overtime Approval" {
		t.Fatalf("create: %+v", p.Create)
	}
	if len(p.Change) != 1 || p.Change[0].ID != 2 || fmt.Sprint(p.Change[0].Fields) != "[name]" || p.Unchanged != 2 {
		t.Fatalf("change: %+v unchanged %d", p.Change, p.Unchanged)
	}
	var codes []string
	for _, pr := range p.Problems {
		codes = append(codes, pr.Code)
	}
	if fmt.Sprint(codes) != "[missing_parent unknown_client]" || len(p.Missing) != 1 || Show(p.Missing[0]) != "HR › Reports" {
		t.Fatalf("problems %v missing %v", codes, p.Missing)
	}
}
