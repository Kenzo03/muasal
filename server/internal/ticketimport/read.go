// Package ticketimport brings tickets in from a CSV file or Jira's "Export
// Excel CSV (all fields)" (FSD §14.2). Read streams the file as records under
// a column mapping; Plan counts a dry run; Apply writes a batch. Imports are
// idempotent: tickets match on their old key per project, comments on a hash
// of key, date, author and body (R-IN-1).
package ticketimport

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"time"
)

// Fields a column can map to.
var Fields = []string{"key", "title", "description", "reason", "type", "status", "priority", "client", "created", "resolved", "reporter", "assignee", "components", "labels", "comments", "attachments"}

// Mapping says which column feeds which field, and how values convert.
type Mapping struct {
	Columns    map[string]string `json:"columns"`              // field → header; repeated headers (Comment, Labels) are all read
	Types      map[string]string `json:"types,omitempty"`      // lower-case source value → bug, change_request or feature
	Statuses   map[string]string `json:"statuses,omitempty"`   // lower-case source value → a project status name
	Priorities map[string]string `json:"priorities,omitempty"` // lower-case source value → low, medium, high or urgent
	Nodes      map[string]int64  `json:"nodes,omitempty"`      // lower-case component or label → node id; others match by name, alias or code
}

// JiraPreset maps Jira's CSV export (§14.2): Story → Feature, Improvement →
// Change request; Components and Labels become menus.
func JiraPreset() Mapping {
	return Mapping{
		Columns: map[string]string{
			"key": "Issue key", "title": "Summary", "description": "Description", "type": "Issue Type", "status": "Status",
			"priority": "Priority", "created": "Created", "resolved": "Resolved", "reporter": "Reporter", "assignee": "Assignee",
			"components": "Component/s", "labels": "Labels", "comments": "Comment", "attachments": "Attachment",
		},
		Types: map[string]string{
			"bug": "bug", "defect": "bug", "incident": "bug",
			"story": "feature", "new feature": "feature", "feature": "feature", "epic": "feature",
			"improvement": "change_request", "task": "change_request", "sub-task": "change_request", "change request": "change_request",
		},
		Priorities: map[string]string{
			"blocker": "urgent", "highest": "urgent", "critical": "urgent", "high": "high", "major": "high",
			"medium": "medium", "minor": "low", "low": "low", "lowest": "low", "trivial": "low",
		},
	}
}

// Record is one row as the mapping reads it.
type Record struct {
	Line                            int
	Key, Title, Description, Reason string
	Type, Status, Priority, Client  string
	Reporter, Assignee              string
	Created, Resolved               *time.Time
	Components, Labels, Attachments []string
	Comments                        []Comment
	Raw                             map[string]string // every non-empty column, for external_meta
}

// Comment is one of Jira's "date;author;body" comment cells.
type Comment struct {
	At     time.Time
	Author string
	Body   string
}

// Headers reads a file's header row, for the mapping screen.
func Headers(r io.Reader) ([]string, error) {
	cr, err := reader(r)
	if err != nil {
		return nil, err
	}
	return cr.Read()
}

// Read streams the file's records under m. each may stop the read by
// returning an error, which Read returns.
func Read(r io.Reader, m Mapping, each func(Record) error) error {
	cr, err := reader(r)
	if err != nil {
		return err
	}
	head, err := cr.Read()
	if err != nil {
		return err
	}
	cols := map[string][]int{} // header → every column index with it
	for i, h := range head {
		cols[strings.TrimSpace(h)] = append(cols[strings.TrimSpace(h)], i)
	}
	all := func(rec []string, field string) []string {
		var out []string
		for _, i := range cols[m.Columns[field]] {
			if i < len(rec) {
				if v := strings.TrimSpace(rec[i]); v != "" {
					out = append(out, v)
				}
			}
		}
		return out
	}
	one := func(rec []string, field string) string {
		if v := all(rec, field); len(v) > 0 {
			return v[0]
		}
		return ""
	}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(strings.Join(rec, "")) == "" {
			continue
		}
		out := Record{
			Line: line, Key: strings.ToUpper(one(rec, "key")), Title: one(rec, "title"), Description: one(rec, "description"),
			Reason: one(rec, "reason"), Type: one(rec, "type"), Status: one(rec, "status"), Priority: one(rec, "priority"),
			Client: one(rec, "client"), Reporter: one(rec, "reporter"), Assignee: one(rec, "assignee"),
			Components: splitList(all(rec, "components")), Labels: splitList(all(rec, "labels")), Attachments: all(rec, "attachments"),
			Created: parseTime(one(rec, "created")), Resolved: parseTime(one(rec, "resolved")), Raw: map[string]string{},
		}
		for _, c := range all(rec, "comments") {
			out.Comments = append(out.Comments, parseComment(c))
		}
		for i, v := range rec {
			if v = strings.TrimSpace(v); v != "" && i < len(head) {
				if prev, ok := out.Raw[head[i]]; ok {
					v = prev + "\n" + v
				}
				out.Raw[head[i]] = v
			}
		}
		if err := each(out); err != nil {
			return err
		}
	}
}

// reader opens a UTF-8 CSV, with or without a byte-order mark, separated by
// commas or, when the header holds more of them, semicolons.
func reader(r io.Reader) (*csv.Reader, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	if b, _ := br.Peek(3); bytes.Equal(b, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	peek, _ := br.Peek(8 << 10)
	first := peek
	if i := bytes.IndexByte(peek, '\n'); i >= 0 {
		first = peek[:i]
	}
	cr := csv.NewReader(br)
	if bytes.Count(first, []byte(";")) > bytes.Count(first, []byte(",")) {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	return cr, nil
}

// splitList splits Labels cells, which Jira separates by spaces within one
// column when it does not repeat the column.
func splitList(vals []string) []string {
	var out []string
	for _, v := range vals {
		for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ';' }) {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}

// Layouts Jira and spreadsheets write dates in.
var layouts = []string{
	"02/Jan/06 3:04 PM", "02/Jan/06 15:04", "2/Jan/06 3:04 PM", "02/Jan/2006 3:04 PM", "02/Jan/2006 15:04",
	time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "02/01/2006 15:04", "02/01/2006", "1/2/2006 15:04", "1/2/2006",
}

func parseTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

// parseComment reads "12/Mar/24 2:05 PM;jdoe;body", Jira's comment cell. A
// cell in another shape is all body.
func parseComment(s string) Comment {
	parts := strings.SplitN(s, ";", 3)
	if len(parts) == 3 {
		if at := parseTime(parts[0]); at != nil {
			return Comment{At: *at, Author: strings.TrimSpace(parts[1]), Body: strings.TrimSpace(parts[2])}
		}
	}
	return Comment{Body: strings.TrimSpace(s)}
}
