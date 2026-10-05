package indexer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/indexer"
)

func ptr[T any](v T) *T { return &v }

func overtimeTicket() indexer.Source {
	created := time.Date(2025, 5, 1, 9, 0, 0, 0, time.UTC)
	closed := time.Date(2025, 6, 10, 9, 0, 0, 0, time.UTC)
	return indexer.Source{
		Ticket: db.GetTicketSourceRow{
			ID: 231, Key: "HRIS-231", Type: "change_request", Title: "Skip supervisor approval for overtime",
			Reason: "Supervisors at Client A are often on leave; HR approves overtime directly.", ProjectID: 1,
			ClientID: ptr[int64](4), ClientName: ptr("Client A"), RequesterContactID: ptr[int64](9),
			ContactName: ptr("Budi"), ContactTitle: ptr("HR Manager"), ContactClientName: ptr("Client A"),
			ReporterID: 3, AssigneeID: ptr[int64](5), CreatedAt: created, ClosedAt: &closed, StatusName: "Done",
		},
		Menus: []db.ListTicketNodePathsRow{{ID: 812, Path: []string{"HR", "Attendance", "Overtime Approval"}}},
		Comments: []db.ListCommentSourcesRow{
			{ID: 77, Author: "Rina", Internal: true, Body: "Confirmed with Budi by phone; applies to all branches.", CreatedAt: created.AddDate(0, 0, 1)},
		},
		Decision: &db.GetDecisionRow{
			DecisionRecord: db.DecisionRecord{
				TicketID: 231, WhatChanged: "Overtime approval skips the supervisor step for Client A.",
				Why: "Approvals stalled for days during leave periods.", Alternatives: "Backup supervisor, rejected.",
				Outcome: "implemented", State: "confirmed", ConfirmedBy: ptr[int64](3), ConfirmedAt: &closed,
			},
			ConfirmerName: ptr("Rina"),
		},
	}
}

// §13.1: a header, a comment and a decision, each opening with the context
// line and carrying the ticket's filter columns.
func TestBuildMakesOneChunkPerSourceWithContext(t *testing.T) {
	chunks := indexer.Build(overtimeTicket())
	if len(chunks) != 3 {
		t.Fatalf("chunks: %d", len(chunks))
	}
	header, comment, decision := chunks[0], chunks[1], chunks[2]
	wantHeader := "HRIS-231 · Client A · Overtime Approval\n" +
		"Change request: Skip supervisor approval for overtime\n" +
		"Status: Done, closed 2025-06-10\n" +
		"Menus: HR › Attendance › Overtime Approval\n" +
		"Requested by: Budi (HR Manager, Client A)\n" +
		"Reason: Supervisors at Client A are often on leave; HR approves overtime directly."
	if header.Content != wantHeader || header.SourceType != "ticket" || !header.OccurredAt.Equal(*overtimeTicket().Ticket.ClosedAt) {
		t.Fatalf("header:\n%s", header.Content)
	}
	if comment.SourceType != "comment" || comment.SourceID != 77 || !comment.Internal ||
		!strings.Contains(comment.Content, "Comment by Rina on 2025-05-02 (Internal):\nConfirmed with Budi") {
		t.Fatalf("comment: %+v", comment)
	}
	if decision.SourceType != "decision" || !strings.Contains(decision.Content, "Decision (implemented), confirmed by Rina on 2025-06-10:\nWhat changed: Overtime approval skips") ||
		!strings.Contains(decision.Content, "Alternatives rejected: Backup supervisor, rejected.") {
		t.Fatalf("decision:\n%s", decision.Content)
	}
	for _, c := range chunks {
		if *c.ClientID != 4 || len(c.NodeIds) != 1 || c.NodeIds[0] != 812 || len(c.UserIds) != 2 || len(c.ContactIds) != 1 || len(c.ContentHash) != 32 {
			t.Fatalf("filter columns: %+v", c)
		}
	}
	if indexer.Key(comment) != "comment:77:0" {
		t.Fatalf("key: %s", indexer.Key(comment))
	}
}

// A draft decision is not indexed; core work reads "All clients".
func TestDraftsAreLeftOutAndCoreWorkIsNamed(t *testing.T) {
	src := overtimeTicket()
	src.Decision.DecisionRecord.State, src.Decision.DecisionRecord.ConfirmedAt = "draft", nil
	src.Ticket.ClientID, src.Ticket.ClientName = nil, nil
	chunks := indexer.Build(src)
	if len(chunks) != 2 || !strings.HasPrefix(chunks[0].Content, "HRIS-231 · All clients · Overtime Approval\n") || chunks[0].ClientID != nil {
		t.Fatalf("chunks: %d %q", len(chunks), chunks[0].Content)
	}
}

// §13.1: a long description splits at about 400 tokens with a 50-token
// overlap, and every part repeats the context line.
func TestLongTextSplitsWithOverlap(t *testing.T) {
	src := overtimeTicket()
	src.Comments, src.Decision = nil, nil
	words := make([]string, 900)
	for i := range words {
		words[i] = "word" + string(rune('a'+i%26))
	}
	src.Ticket.Description = strings.Join(words, " ")
	chunks := indexer.Build(src)
	if len(chunks) < 3 {
		t.Fatalf("parts: %d", len(chunks))
	}
	for i, c := range chunks {
		body := strings.TrimPrefix(c.Content, "HRIS-231 · Client A · Overtime Approval\n")
		if body == c.Content || len([]rune(body)) > 1400 || c.Seq != int32(i) {
			t.Fatalf("part %d: %d chars, seq %d", i, len([]rune(body)), c.Seq)
		}
		if i > 0 {
			prev := strings.TrimPrefix(chunks[i-1].Content, "HRIS-231 · Client A · Overtime Approval\n")
			if !strings.Contains(prev, body[:60]) {
				t.Fatalf("part %d does not overlap the one before", i)
			}
		}
	}
}
