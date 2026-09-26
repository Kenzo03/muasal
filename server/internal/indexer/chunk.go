// Package indexer keeps the retrieval index in step with tickets: it turns a
// ticket, its comments and its decision record into chunks, embeds changed
// chunks, and runs as River jobs queued in the same transaction as each change
// (FSD §13).
package indexer

import (
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Chunk sizes (§13.1): about 400 tokens with a 50-token overlap, with tokens
// estimated as characters ÷ 3.5 (§11.4).
const (
	maxChars     = 1400
	overlapChars = 175
)

// Source is one ticket as its chunks describe it. Decision is nil unless the
// record is confirmed: a draft is not a decision yet (R-DC-6).
type Source struct {
	Ticket   db.GetTicketSourceRow
	Menus    []db.ListTicketNodePathsRow
	Comments []db.ListCommentSourcesRow
	Decision *db.GetDecisionRow
	Commits  []db.ListTicketCommitsRow       // linked by Git webhooks (§14.1)
	MRs      []db.ListTicketMergeRequestsRow //
	Coders   []int64                         // users whose email matches a commit author: the Person filter finds them
}

var typeLabels = map[string]string{"bug": "Bug", "change_request": "Change request", "feature": "Feature"}

// Build turns a ticket into its chunks: the header, one source per comment and
// the decision record. Every chunk starts with a context line such as
// "HRIS-231 · Client A · Overtime Approval", so vector and keyword hits both
// carry context, and every chunk carries the ticket's filter columns.
func Build(src Source) []db.UpsertChunkParams {
	t := src.Ticket
	var out []db.UpsertChunkParams
	add := func(sourceType string, sourceID int64, at time.Time, internal bool, body string) {
		users := userIDs(t)
		if sourceType == "code" {
			users = slices.Compact(slices.Sorted(slices.Values(append(users, src.Coders...))))
		}
		for i, part := range split(body) {
			content := contextLine(src) + "\n" + part
			sum := sha256.Sum256([]byte(content))
			out = append(out, db.UpsertChunkParams{
				SourceType: sourceType, SourceID: sourceID, Seq: int32(i), TicketID: &t.ID, ProjectID: t.ProjectID,
				ClientID: t.ClientID, NodeIds: menuIDs(src), UserIds: users, ContactIds: contactIDs(t),
				Internal: internal, OccurredAt: at, Content: content, ContentHash: sum[:],
			})
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n", typeLabels[t.Type], t.Title)
	if t.ExternalRef != nil {
		b.WriteString("Imported, old key " + *t.ExternalRef + "\n") // so the old key finds it too (R-IN-2)
	}
	b.WriteString("Status: " + t.StatusName)
	if t.ClosedAt != nil {
		b.WriteString(", closed " + day(*t.ClosedAt))
	} else {
		b.WriteString(", created " + day(t.CreatedAt))
	}
	b.WriteString("\n")
	if len(src.Menus) > 0 {
		paths := make([]string, len(src.Menus))
		for i, m := range src.Menus {
			paths[i] = strings.Join(m.Path, " › ")
		}
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	b.WriteString("Requested by: " + Requester(t) + "\n")
	if s := strings.TrimSpace(t.Reason); s != "" {
		b.WriteString("Reason: " + s + "\n")
	}
	if s := strings.TrimSpace(t.Description); s != "" {
		b.WriteString("Description: " + s + "\n")
	}
	at := t.CreatedAt
	if t.ClosedAt != nil {
		at = *t.ClosedAt
	}
	add("ticket", t.ID, at, false, strings.TrimSpace(b.String()))

	for _, c := range src.Comments {
		label := ""
		if c.Internal {
			label = " (Internal)"
		}
		add("comment", c.ID, c.CreatedAt, c.Internal, fmt.Sprintf("Comment by %s on %s%s:\n%s", c.Author, day(c.CreatedAt), label, strings.TrimSpace(c.Body)))
	}

	if d := src.Decision; d != nil && d.DecisionRecord.ConfirmedAt != nil {
		r := d.DecisionRecord
		var b strings.Builder
		fmt.Fprintf(&b, "Decision (%s), confirmed by %s on %s:\n", r.Outcome, deref(d.ConfirmerName), day(*r.ConfirmedAt))
		b.WriteString("What changed: " + r.WhatChanged + "\n")
		b.WriteString("Why: " + r.Why)
		if s := strings.TrimSpace(r.Alternatives); s != "" {
			b.WriteString("\nAlternatives rejected: " + s)
		}
		if d.SupersededByKey != nil {
			b.WriteString("\nSuperseded by " + *d.SupersededByKey + ": no longer how it works")
		}
		add("decision", t.ID, *r.ConfirmedAt, false, b.String())
	}
	if code := CodeText(src); code != "" {
		add("code", t.ID, at, false, code)
	}
	return out
}

// CodeText is a ticket's merge requests and commits as a chunk and as
// evidence reads them (§13.1): repository, number or short SHA, author, date
// and title or message.
func CodeText(src Source) string {
	var b strings.Builder
	for _, m := range src.MRs {
		fmt.Fprintf(&b, "Merge request %s !%d (%s): %s\n", m.RepoName, m.Number, m.State, m.Title)
	}
	for _, c := range src.Commits {
		sha := c.Sha[:min(7, len(c.Sha))]
		when := ""
		if c.CommittedAt != nil {
			when = " on " + day(*c.CommittedAt)
		}
		fmt.Fprintf(&b, "Commit %s %s by %s%s: %s\n", c.RepoName, sha, deref(c.AuthorName), when, strings.TrimSpace(c.Message))
	}
	return strings.TrimSpace(b.String())
}

// Key names a chunk as DeleteStaleChunks expects it.
func Key(c db.UpsertChunkParams) string {
	return fmt.Sprintf("%s:%d:%d", c.SourceType, c.SourceID, c.Seq)
}

// contextLine is "HRIS-231 · Client A · Overtime Approval"; core work reads "All clients".
func contextLine(src Source) string {
	parts := []string{src.Ticket.Key, "All clients"}
	if src.Ticket.ClientName != nil {
		parts[1] = *src.Ticket.ClientName
	}
	for _, m := range src.Menus {
		parts = append(parts, m.Path[len(m.Path)-1])
	}
	return strings.Join(parts, " · ")
}

// Requester is "Budi (HR Manager, Client A)" for a contact, or the user's name.
func Requester(t db.GetTicketSourceRow) string {
	if t.ContactName == nil {
		return deref(t.RequesterUserName)
	}
	var about []string
	if t.ContactTitle != nil && *t.ContactTitle != "" {
		about = append(about, *t.ContactTitle)
	}
	if t.ContactClientName != nil {
		about = append(about, *t.ContactClientName)
	}
	if len(about) == 0 {
		return *t.ContactName
	}
	return *t.ContactName + " (" + strings.Join(about, ", ") + ")"
}

// split cuts text into parts of at most maxChars, at whitespace, with each
// part repeating the last overlapChars of the one before.
func split(text string) []string {
	r := []rune(text)
	if len(r) <= maxChars {
		return []string{text}
	}
	var parts []string
	for start := 0; start < len(r); {
		end := min(start+maxChars, len(r))
		if end < len(r) {
			for i := end; i > start+maxChars/2; i-- {
				if unicode.IsSpace(r[i]) {
					end = i
					break
				}
			}
		}
		parts = append(parts, strings.TrimSpace(string(r[start:end])))
		if end == len(r) {
			break
		}
		next := end - overlapChars
		for next < end && !unicode.IsSpace(r[next]) {
			next++
		}
		start = next
	}
	return parts
}

func menuIDs(src Source) []int64 {
	ids := make([]int64, len(src.Menus))
	for i, m := range src.Menus {
		ids[i] = m.ID
	}
	return ids
}

func userIDs(t db.GetTicketSourceRow) []int64 {
	ids := []int64{t.ReporterID}
	for _, id := range []*int64{t.RequesterUserID, t.AssigneeID} {
		if id != nil {
			ids = append(ids, *id)
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func contactIDs(t db.GetTicketSourceRow) []int64 {
	if t.RequesterContactID == nil {
		return []int64{}
	}
	return []int64{*t.RequesterContactID}
}

func day(t time.Time) string { return t.UTC().Format(time.DateOnly) }

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
