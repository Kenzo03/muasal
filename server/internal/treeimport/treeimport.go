// Package treeimport reads a module-tree CSV and plans its import against a
// project's tree (FSD §7.5, MR-4): new, changed and unchanged nodes, row
// errors, and the nodes the file leaves out. It touches no database; the API
// applies the plan.
package treeimport

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strings"
)

// MaxRows bounds one file.
const MaxRows = 5000

// Row is one node as the file describes it. Path runs from the top of the tree.
type Row struct {
	Line           int
	Path           []string
	Type           string // module or menu
	Code           string
	ClientSpecific bool
	Clients        []string
	Aliases        []string
}

// Problem is a row error; Line 0 is the file as a whole. Code lets the UI
// translate; Path names the parent or the row, joined with " › ".
type Problem struct {
	Line    int    `json:"line"`
	Code    string `json:"code"`
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

// Parse reads the path shape (path,type,code,client_scope,clients,aliases)
// or the adjacency shape (id,parent_id,name,type,code), separated by commas or
// semicolons, UTF-8 with or without a byte-order mark.
func Parse(r io.Reader) ([]Row, []Problem, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	first, _, _ := bufio.NewReader(bytes.NewReader(raw)).ReadLine()
	cr := csv.NewReader(bytes.NewReader(raw))
	if strings.Count(string(first), ";") > strings.Count(string(first), ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, []Problem{{Code: "unreadable", Message: "The file is not readable CSV: " + err.Error()}}, nil
	}
	if len(records) < 2 {
		return nil, []Problem{{Code: "empty", Message: "The file has no rows under its header"}}, nil
	}
	if len(records)-1 > MaxRows {
		return nil, []Problem{{Code: "too_many_rows", Message: fmt.Sprintf("Import at most %d rows at a time", MaxRows)}}, nil
	}
	head := map[string]int{}
	for i, h := range records[0] {
		head[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(rec []string, col string) string {
		if i, ok := head[col]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	_, pathShape := head["path"]
	_, idShape := head["id"]
	switch {
	case pathShape:
		return parsePaths(records[1:], get)
	case idShape:
		if _, ok := head["parent_id"]; !ok {
			break
		}
		return parseAdjacency(records[1:], get)
	}
	return nil, []Problem{{Code: "unknown_shape", Message: "Use the columns path,type,code,client_scope,clients,aliases or id,parent_id,name,type,code"}}, nil
}

func parsePaths(records [][]string, get func([]string, string) string) ([]Row, []Problem, error) {
	var rows []Row
	var problems []Problem
	for i, rec := range records {
		line := i + 2
		if strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue
		}
		var path []string
		for _, part := range strings.Split(strings.ReplaceAll(get(rec, "path"), "›", ">"), ">") {
			path = append(path, strings.Join(strings.Fields(part), " "))
		}
		row := Row{Line: line, Path: path, Type: strings.ToLower(get(rec, "type")), Code: get(rec, "code"), Clients: list(get(rec, "clients")), Aliases: list(get(rec, "aliases"))}
		switch strings.ToLower(get(rec, "client_scope")) {
		case "", "shared":
		case "client_specific":
			row.ClientSpecific = true
		default:
			problems = append(problems, Problem{Line: line, Code: "bad_client_scope", Message: "client_scope is shared or client_specific"})
			continue
		}
		rows = append(rows, row)
	}
	return rows, problems, nil
}

func parseAdjacency(records [][]string, get func([]string, string) string) ([]Row, []Problem, error) {
	type item struct {
		line              int
		parent, name, typ string
		code              string
	}
	byID := map[string]item{}
	var order []string
	var problems []Problem
	for i, rec := range records {
		line := i + 2
		id := get(rec, "id")
		if id == "" {
			if strings.TrimSpace(strings.Join(rec, "")) != "" {
				problems = append(problems, Problem{Line: line, Code: "missing_id", Message: "Every row needs an id"})
			}
			continue
		}
		if _, dup := byID[id]; dup {
			problems = append(problems, Problem{Line: line, Code: "duplicate_id", Message: "The id " + id + " appears twice"})
			continue
		}
		byID[id] = item{line: line, parent: get(rec, "parent_id"), name: strings.Join(strings.Fields(get(rec, "name")), " "), typ: strings.ToLower(get(rec, "type")), code: get(rec, "code")}
		order = append(order, id)
	}
	var rows []Row
	for _, id := range order {
		it := byID[id]
		path := []string{it.name}
		seen := map[string]bool{id: true}
		ok := true
		for p := it.parent; p != "" && p != "0"; {
			up, found := byID[p]
			if !found {
				problems = append(problems, Problem{Line: it.line, Code: "missing_parent", Path: p, Message: "No row has the parent id " + p})
				ok = false
				break
			}
			if seen[p] {
				problems = append(problems, Problem{Line: it.line, Code: "cycle", Message: "The parent ids of this row loop back to it"})
				ok = false
				break
			}
			seen[p] = true
			path = append([]string{up.name}, path...)
			p = up.parent
		}
		if ok {
			rows = append(rows, Row{Line: it.line, Path: path, Type: it.typ, Code: it.code})
		}
	}
	return rows, problems, nil
}

func list(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Existing is a node of the project's tree, archived ones included.
type Existing struct {
	ID             int64
	Path           []string
	Type           string
	Code           string
	ClientSpecific bool
	Clients        []string
	Aliases        []string
	Archived       bool
}

// Change is a row that matches an existing node whose fields differ.
type Change struct {
	Row    Row
	ID     int64
	Fields []string // type, code, aliases, clients, name, parent
}

// Plan is what an import would do.
type Plan struct {
	Create    []Row
	Change    []Change
	Unchanged int
	Missing   [][]string // live nodes the file leaves out, for manual archiving
	Problems  []Problem
}

// Show is a path as the UI shows it: "HR › Attendance".
func Show(path []string) string { return strings.Join(path, " › ") }

func pathKey(path []string) string { return strings.ToLower(strings.Join(path, "\x00")) }

// Diff plans rows against the tree. Rows match nodes by code, else by full
// path, ignoring case; nothing is ever deleted (§7.5). clients are the
// project's clients by name.
func Diff(rows []Row, existing []Existing, clients []string) Plan {
	var p Plan
	byCode := map[string]Existing{}
	byPath := map[string]Existing{}
	for _, e := range existing {
		if e.Code != "" {
			byCode[e.Code] = e
		}
		if !e.Archived {
			byPath[pathKey(e.Path)] = e
		}
	}
	inFile := map[string]int{} // path → line
	codes := map[string]int{}
	for _, r := range rows {
		k := pathKey(r.Path)
		if prev, dup := inFile[k]; dup {
			p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "duplicate_name", Path: Show(r.Path[:len(r.Path)-1]),
				Message: fmt.Sprintf("Duplicate name under %s (also row %d)", parentName(r.Path), prev)})
			continue
		}
		inFile[k] = r.Line
		if r.Code != "" {
			if prev, dup := codes[r.Code]; dup {
				p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "duplicate_code", Message: fmt.Sprintf("The code %s is also on row %d", r.Code, prev)})
			}
			codes[r.Code] = r.Line
		}
	}
	matched := map[int64]bool{}
	for _, r := range rows {
		switch {
		case slices.ContainsFunc(r.Path, func(s string) bool { return s == "" }) || len(r.Path) == 0:
			p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "empty_name", Message: "A path has an empty name"})
			continue
		case r.Type != "module" && r.Type != "menu":
			p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "bad_type", Message: "type is module or menu"})
			continue
		case slices.ContainsFunc(r.Path, func(s string) bool { return len([]rune(s)) > 100 }):
			p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "long_name", Message: "Names take at most 100 characters"})
			continue
		}
		if parent := r.Path[:len(r.Path)-1]; len(parent) > 0 {
			if _, inTree := byPath[pathKey(parent)]; !inTree {
				if _, ok := inFile[pathKey(parent)]; !ok {
					p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "missing_parent", Path: Show(parent), Message: "Missing parent " + Show(parent)})
					continue
				}
			}
		}
		unknown := false
		for _, c := range r.Clients {
			if !slices.ContainsFunc(clients, func(x string) bool { return strings.EqualFold(x, c) }) {
				p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "unknown_client", Path: c, Message: "Unknown client " + c})
				unknown = true
			}
		}
		if unknown {
			continue
		}
		if r.ClientSpecific && len(r.Clients) == 0 {
			p.Problems = append(p.Problems, Problem{Line: r.Line, Code: "no_clients", Message: "A client-specific node names at least one client"})
			continue
		}
		e, ok := byCode[r.Code]
		if r.Code == "" || !ok {
			e, ok = byPath[pathKey(r.Path)]
		}
		if !ok {
			p.Create = append(p.Create, r)
			continue
		}
		matched[e.ID] = true
		if f := changed(r, e); len(f) > 0 {
			p.Change = append(p.Change, Change{Row: r, ID: e.ID, Fields: f})
		} else {
			p.Unchanged++
		}
	}
	for _, e := range existing {
		if !e.Archived && !matched[e.ID] {
			p.Missing = append(p.Missing, e.Path)
		}
	}
	slices.SortFunc(p.Problems, func(a, b Problem) int { return a.Line - b.Line })
	return p
}

func parentName(path []string) string {
	if len(path) < 2 {
		return "the top level"
	}
	return Show(path[:len(path)-1])
}

func changed(r Row, e Existing) []string {
	var f []string
	if !strings.EqualFold(r.Path[len(r.Path)-1], e.Path[len(e.Path)-1]) {
		f = append(f, "name")
	}
	if pathKey(r.Path[:len(r.Path)-1]) != pathKey(e.Path[:len(e.Path)-1]) {
		f = append(f, "parent")
	}
	if r.Type != e.Type {
		f = append(f, "type")
	}
	if r.Code != "" && r.Code != e.Code {
		f = append(f, "code")
	}
	if len(r.Aliases) > 0 && !sameSet(r.Aliases, e.Aliases) {
		f = append(f, "aliases")
	}
	if r.ClientSpecific != e.ClientSpecific || (r.ClientSpecific && !sameSet(r.Clients, e.Clients)) {
		f = append(f, "clients")
	}
	return f
}

func sameSet(a, b []string) bool {
	low := func(xs []string) []string {
		out := make([]string, len(xs))
		for i, x := range xs {
			out[i] = strings.ToLower(x)
		}
		slices.Sort(out)
		return slices.Compact(out)
	}
	return slices.Equal(low(a), low(b))
}
