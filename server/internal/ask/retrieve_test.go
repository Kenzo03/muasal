package ask_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
)

var hybrid = ask.Tuning{EmbedModel: "bge-m3", MinSimilarity: 0.3, ExhaustiveMax: 0} // always rank

func (w *world) retrieve(u db.User, s ask.Scope, question string, t ask.Tuning) ask.Found {
	w.t.Helper()
	vec := llmtest.Vector(question, 1024)
	found, err := ask.Retrieve(context.Background(), w.d.Pool, w.asker(u), s, question, vec, ask.Detect(ask.Catalog{}, question, now).Keys, t)
	w.check(err)
	return found
}

func keysOf(w *world, ids []int64) []string {
	var out []string
	for _, id := range ids {
		out = append(out, must(w.q.GetTicketSource(context.Background(), id)).Key)
	}
	return out
}

// AC-AK-3: a member scoped to Client A never gets Client B's tickets as
// evidence, however closely they match.
func TestRetrievalNeverCrossesTheClientScope(t *testing.T) {
	w := newWorld(t)
	forA := w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Client A supervisors are on leave.", "2025-06-10", "Skip the supervisor for Client A.")
	forB := w.ticket("Overtime approval needs two supervisors", &w.b, w.ot, "Client B wants two approvers.", "2025-07-01", "Two supervisors approve Client B overtime.")
	q := "Why does overtime approval need two supervisors for Client B?"
	if got := w.retrieve(w.admin, ask.Scope{}, q, hybrid).TicketIDs; !slices.Contains(got, forB.ID) {
		t.Fatalf("the admin should find Client B's ticket: %v", keysOf(w, got))
	}
	got := w.retrieve(w.member, ask.Scope{ClientIDs: []int64{w.b.ID}}, q, hybrid)
	if slices.Contains(got.TicketIDs, forB.ID) || slices.Contains(got.Closest, forB.ID) {
		t.Fatalf("Client B's ticket reached the member: %v %v", keysOf(w, got.TicketIDs), keysOf(w, got.Closest))
	}
	named := w.retrieve(w.member, ask.Scope{}, "What did "+forB.Key+" and "+forA.Key+" change?", hybrid)
	if slices.Contains(named.TicketIDs, forB.ID) || named.TicketIDs[0] != forA.ID {
		t.Fatalf("keys: %v", keysOf(w, named.TicketIDs))
	}
}

// AC-AK-4: a date range keeps evidence dated inside it: the close date, or
// the creation date while open.
func TestRetrievalKeepsToTheDateRange(t *testing.T) {
	w := newWorld(t)
	w.ticket("Overtime cap of 40 hours", &w.a, w.ot, "Labor agreement caps overtime.", "2025-12-20", "Cap overtime at 40 hours.")
	inQ1 := w.ticket("Overtime cap raised to 50 hours", &w.a, w.ot, "New labor agreement.", "2026-02-10", "Cap overtime at 50 hours.")
	w.ticket("Overtime cap for interns", &w.a, w.ot, "Interns work fewer hours.", "2026-04-02", "Cap intern overtime at 10 hours.")
	got := w.retrieve(w.admin, ask.Scope{From: day("2026-01-01"), To: day("2026-03-31")}, "What is the overtime cap?", hybrid)
	if !slices.Equal(got.TicketIDs, []int64{inQ1.ID}) {
		t.Fatalf("evidence: %v", keysOf(w, got.TicketIDs))
	}
}

// §11.3: with no keyword hit and nothing similar enough, there is no evidence,
// though the closest tickets are still named.
func TestTheRelevanceFloorLeavesNoEvidence(t *testing.T) {
	w := newWorld(t)
	w.ticket("Overtime approval skips supervisor", &w.a, w.ot, "Supervisors are on leave.", "", "")
	strict := hybrid
	strict.MinSimilarity = 0.99
	got := w.retrieve(w.admin, ask.Scope{}, "Quantum zebra marmalade?", strict)
	if len(got.TicketIDs) != 0 {
		t.Fatalf("evidence below the floor: %v", keysOf(w, got.TicketIDs))
	}
}

// §11.3: a small scope takes every item, the most relevant first; a larger
// one takes the best-ranked tickets.
func TestSmallScopesTakeEveryItem(t *testing.T) {
	w := newWorld(t)
	old := w.ticket("Overtime export format", &w.a, w.ot, "Payroll imports a CSV.", "2025-01-15", "Export overtime as CSV.")
	recent := w.ticket("Overtime approval by HR", &w.a, w.ot, "Supervisors are on leave.", "2026-05-01", "HR approves overtime.")
	payroll := w.ticket("Payslip shows overtime", nil, w.hr, "Employees asked for detail.", "2026-03-01", "Payslips list overtime hours.")
	q := "Why does HR approve overtime?"
	small := w.retrieve(w.admin, ask.Scope{NodeIDs: []int64{w.ot.ID}}, q, ask.Tuning{EmbedModel: "bge-m3", MinSimilarity: 0.3, ExhaustiveMax: 40})
	if !small.Exhaustive || len(small.TicketIDs) != 2 || small.TicketIDs[0] != recent.ID || !slices.Contains(small.TicketIDs, old.ID) {
		t.Fatalf("small set: %v %v", small.Exhaustive, keysOf(w, small.TicketIDs))
	}
	ranked := w.retrieve(w.admin, ask.Scope{}, q, hybrid)
	if ranked.Exhaustive || ranked.TicketIDs[0] != recent.ID || !slices.Contains(ranked.TicketIDs, payroll.ID) {
		t.Fatalf("ranked: %v", keysOf(w, ranked.TicketIDs))
	}
}

// §11.4: over budget, the lowest-ranked ticket loses its comments first, then goes.
func TestPackKeepsTheBudget(t *testing.T) {
	w := newWorld(t)
	var items []indexer.Source
	for _, title := range []string{"Overtime approval by HR", "Overtime export format", "Overtime cap"} {
		tk := w.ticket(title, &w.a, w.ot, "Supervisors are on leave.", "2026-05-01", "HR approves overtime.")
		_, err := w.d.Pool.Exec(context.Background(), `INSERT INTO comments (ticket_id, author_id, body) VALUES ($1, $2, $3)`,
			tk.ID, w.admin.ID, strings.Repeat("Discussed with the client at length. ", 8))
		w.check(err)
		items = append(items, must(indexer.Load(context.Background(), w.q, tk.ID)))
	}
	text, keys := ask.Pack(items, 10000)
	if len(keys) != 3 || !strings.Contains(text, "[HRIS-1] Change request · Client A · closed 2026-05-01 as Done · requested by Hana\nTitle: Overtime approval by HR\nMenus: HR › Attendance › Overtime Approval\nReason: Supervisors are on leave.\nDecision (implemented): HR approves overtime.") ||
		strings.Count(text, "Comment ") != 3 {
		t.Fatalf("roomy budget:\n%s", text)
	}
	text, keys = ask.Pack(items, 180) // room for two tickets without comments
	if !slices.Equal(keys, []string{"HRIS-1", "HRIS-2"}) || strings.Count(text, "Comment ") != 0 {
		t.Fatalf("tight budget: %v\n%s", keys, text)
	}
}
