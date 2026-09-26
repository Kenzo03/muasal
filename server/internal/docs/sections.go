// Package docs reads project documents into sections and drafts module trees
// from them (FSD §7.7): from the model when AI is on, from the headings when
// it is off, merged without a model call and reviewed by an admin.
package docs

import (
	"fmt"
	"regexp"
	"strings"
)

// Section is one heading of a document and the text under it.
type Section struct {
	Number string // "7.4" from the heading, or a running "s3"
	Title  string
	Level  int // 1 for "#"
	Body   string
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
	numberRe  = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+(.+)$`)
	fenceRe   = regexp.MustCompile("^\\s*(```|~~~)")
)

// Split cuts Markdown at its headings. Text before the first heading becomes
// section "0" when there is any. A heading such as "7.4 Behaviors by client"
// keeps its number; others get a running "s<n>". Numbers stay unique.
func Split(markdown, title string) []Section {
	var out []Section
	var body []string
	cur := Section{Number: "0", Title: title, Level: 1}
	seen := map[string]int{}
	flush := func() {
		cur.Body = strings.TrimSpace(strings.Join(body, "\n"))
		if cur.Number != "0" || cur.Body != "" {
			out = append(out, cur)
		}
		body = nil
	}
	inFence := false
	running := 0
	for _, line := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
		}
		m := headingRe.FindStringSubmatch(line)
		if inFence || m == nil {
			body = append(body, line)
			continue
		}
		flush()
		text := strings.TrimSpace(strings.Trim(m[2], "*_"))
		running++
		number, name := fmt.Sprintf("s%d", running), text
		if n := numberRe.FindStringSubmatch(text); n != nil {
			number, name = n[1], n[2]
		}
		if seen[number]++; seen[number] > 1 {
			number = fmt.Sprintf("%s-%d", number, seen[number])
		}
		cur = Section{Number: number, Title: name, Level: len(m[1])}
	}
	flush()
	return out
}

// Parts groups sections, in order, into parts of at most budget characters
// (§7.7). A section alone over the budget is split at paragraph ends, or
// hard cut when a paragraph is itself too long.
func Parts(sections []Section, budget int) [][]Section {
	budget = max(budget, 500)
	var out [][]Section
	var cur []Section
	size := 0
	size0 := func(s Section) int { return len([]rune(s.Title)) + len([]rune(s.Body)) + 20 }
	for _, s := range sections {
		for _, piece := range splitSection(s, budget) {
			if n := size0(piece); size+n > budget && len(cur) > 0 {
				out = append(out, cur)
				cur, size = nil, 0
			}
			cur = append(cur, piece)
			size += size0(piece)
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func splitSection(s Section, budget int) []Section {
	limit := budget - len([]rune(s.Title)) - 20
	if len([]rune(s.Body)) <= limit {
		return []Section{s}
	}
	var out []Section
	var cur strings.Builder
	emit := func() {
		if cur.Len() > 0 {
			p := s
			p.Body = strings.TrimSpace(cur.String())
			out = append(out, p)
			cur.Reset()
		}
	}
	for _, para := range strings.Split(s.Body, "\n\n") {
		for r := []rune(para); len(r) > 0; {
			take := min(len(r), limit)
			if len([]rune(cur.String()))+take > limit {
				emit()
			}
			cur.WriteString(string(r[:take]) + "\n\n")
			r = r[take:]
		}
	}
	emit()
	return out
}
