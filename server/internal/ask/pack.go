package ask

import (
	"fmt"
	"strings"
	"time"

	"github.com/kenzo03/muasal/server/internal/indexer"
)

var typeNames = map[string]string{"bug": "Bug", "change_request": "Change request", "feature": "Feature"}

// Evidence is one packed item: a ticket, a decision note or a document section.
type Evidence struct {
	Ticket  *indexer.Source
	Note    *indexer.NoteSource
	Section *indexer.SectionSource
}

// Key is the item's citation key, e.g. HRIS-231, HRIS-DN7 or HRIS-DOC1/7.4.
func (e Evidence) Key() string {
	if e.Note != nil {
		return e.Note.Note.DecisionNote.Key
	}
	if e.Section != nil {
		return e.Section.Key()
	}
	return e.Ticket.Ticket.Key
}

// wholeItems is how many of the best-ranked items keep all their details
// while lower-ranked items give way.
// ponytail: a fixed three; tune it if long tickets crowd out breadth.
const wholeItems = 3

// Pack writes the evidence the model sees, one block per item with its most
// useful facts first (§11.4), within budget tokens, estimated as characters ÷
// 3.5. Over budget, the items below the best three lose their comments,
// description and note body first, lowest ranked first, then go whole; only
// then do the best three lose theirs. So the item that answers keeps all of
// it, such as a meeting note's follow-ups at the end of its body. It returns
// the text, the keys packed in order (only those may be cited) and each
// packed key's block.
func Pack(items []Evidence, budgetTokens int) (string, []string, map[string]string) {
	budget := int(float64(budgetTokens) * 3.5)
	full := make([]string, len(items))
	core := make([]string, len(items))
	for i, it := range items {
		if it.Note != nil {
			core[i], full[i] = noteBlock(*it.Note, false), noteBlock(*it.Note, true)
		} else if it.Section != nil {
			core[i], full[i] = sectionBlock(*it.Section, false), sectionBlock(*it.Section, true)
		} else {
			core[i], full[i] = block(*it.Ticket, false), block(*it.Ticket, true)
		}
	}
	use := append([]string(nil), full...)
	size := func() int {
		n := 0
		for _, b := range use {
			n += len([]rune(b)) + 2
		}
		return n
	}
	best := min(wholeItems, len(use))
	for i := len(use) - 1; i >= best && size() > budget; i-- {
		use[i] = core[i]
	}
	for i := len(use) - 1; i >= best && size() > budget; i-- {
		use[i] = ""
	}
	for i := best - 1; i >= 0 && size() > budget; i-- {
		use[i] = core[i]
	}
	for i := best - 1; i > 0 && size() > budget; i-- {
		use[i] = ""
	}
	var text strings.Builder
	var keys []string
	blocks := map[string]string{}
	for i, b := range use {
		if b == "" {
			continue
		}
		text.WriteString(b)
		text.WriteString("\n\n")
		keys = append(keys, items[i].Key())
		blocks[items[i].Key()] = b
	}
	return strings.TrimSpace(text.String()), keys, blocks
}

// block is one ticket's evidence; with details, its description and newest
// five comments too.
func block(it indexer.Source, details bool) string {
	t := it.Ticket
	var b strings.Builder
	client := "All clients"
	if t.ClientName != nil {
		client = *t.ClientName
	}
	when := "open, created " + day(t.CreatedAt) + ", status " + t.StatusName
	if t.ClosedAt != nil {
		when = "closed " + day(*t.ClosedAt) + " as " + t.StatusName
	}
	key := "[" + t.Key + "]"
	if t.ExternalRef != nil {
		key += " (old key " + *t.ExternalRef + ")" // R-IN-2
	}
	fmt.Fprintf(&b, "%s %s · %s · %s · requested by %s\n", key, typeNames[t.Type], client, when, indexer.Requester(t))
	b.WriteString("Title: " + t.Title + "\n")
	if len(it.Menus) > 0 {
		paths := make([]string, len(it.Menus))
		for i, m := range it.Menus {
			paths[i] = strings.Join(m.Path, " › ")
		}
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	if s := strings.TrimSpace(t.Reason); s != "" {
		b.WriteString("Reason: " + s + "\n")
	}
	if d := it.Decision; d != nil {
		r := d.DecisionRecord
		fmt.Fprintf(&b, "Decision (%s): %s\nWhy: %s\n", r.Outcome, r.WhatChanged, r.Why)
		if s := strings.TrimSpace(r.Alternatives); s != "" {
			b.WriteString("Alternatives rejected: " + s + "\n")
		}
		if d.SupersededByKey != nil {
			b.WriteString("Superseded by " + *d.SupersededByKey + "\n")
		}
	}
	if details {
		if s := strings.TrimSpace(t.Description); s != "" {
			b.WriteString("Description: " + cut(s, 500) + "\n")
		}
		if code := indexer.CodeText(it); code != "" {
			b.WriteString(cut(code, 600) + "\n")
		}
		comments := it.Comments[max(0, len(it.Comments)-5):]
		for _, c := range comments {
			fmt.Fprintf(&b, "Comment %s %s: %s\n", day(c.CreatedAt), c.Author, cut(strings.Join(strings.Fields(c.Body), " "), 300))
		}
	}
	return strings.TrimSpace(b.String())
}

// noteBlock is one decision note's evidence; with details, its body too.
func noteBlock(it indexer.NoteSource, details bool) string {
	n := it.Note.DecisionNote
	var b strings.Builder
	client := "All clients"
	if it.Note.ClientName != nil {
		client = *it.Note.ClientName
	}
	fmt.Fprintf(&b, "[%s] Decision note · %s · decided %s", n.Key, client, day(n.DecidedOn))
	if s := strings.TrimSpace(n.Attendees); s != "" {
		b.WriteString(" with " + s)
	}
	b.WriteString("\nTitle: " + n.Title + "\n")
	if len(it.Menus) > 0 {
		paths := make([]string, len(it.Menus))
		for i, m := range it.Menus {
			paths[i] = strings.Join(m.Path, " › ")
		}
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	body := strings.TrimSpace(n.Body)
	if !details {
		body = cut(body, 400)
	}
	b.WriteString(body)
	return strings.TrimSpace(b.String())
}

// sectionBlock is one document section's evidence; with details, more of its body.
func sectionBlock(it indexer.SectionSource, details bool) string {
	s := it.Section
	var b strings.Builder
	client := "All clients"
	if s.ClientName != nil {
		client = *s.ClientName
	}
	fmt.Fprintf(&b, "[%s] Document section · %s · uploaded %s", it.Key(), client, day(s.DocumentCreatedAt))
	if s.SupersededByKey != nil {
		b.WriteString(" · superseded by " + *s.SupersededByKey + ", history only")
	}
	fmt.Fprintf(&b, "\nTitle: %s › %s %s\n", s.DocumentTitle, s.Number, s.Title)
	if len(it.Menus) > 0 {
		paths := make([]string, len(it.Menus))
		for i, m := range it.Menus {
			paths[i] = strings.Join(m.Path, " › ")
		}
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	body := strings.TrimSpace(s.Body)
	if details {
		body = cut(body, 1500)
	} else {
		body = cut(body, 400)
	}
	b.WriteString(body)
	return strings.TrimSpace(b.String())
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func day(t time.Time) string { return t.UTC().Format(time.DateOnly) }
