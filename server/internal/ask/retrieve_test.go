package ask_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

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

func idsOfRefs(refs []ask.Ref) []int64 {
	var out []int64
	for _, r := range refs {
		out = append(out, r.ID)
	}
	return out
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
	if got := w.retrieve(w.admin, ask.Scope{}, q, hybrid).TicketIDs(); !slices.Contains(got, forB.ID) {
		t.Fatalf("the admin should find Client B's ticket: %v", keysOf(w, got))
	}
	got := w.retrieve(w.member, ask.Scope{ClientIDs: []int64{w.b.ID}}, q, hybrid)
	if slices.Contains(got.TicketIDs(), forB.ID) || slices.Contains(got.Closest, ask.Ref{ID: forB.ID}) {
		t.Fatalf("Client B's ticket reached the member: %v %v", keysOf(w, got.TicketIDs()), keysOf(w, idsOfRefs(got.Closest)))
	}
	named := w.retrieve(w.member, ask.Scope{}, "What did "+forB.Key+" and "+forA.Key+" change?", hybrid)
	if slices.Contains(named.TicketIDs(), forB.ID) || named.TicketIDs()[0] != forA.ID {
		t.Fatalf("keys: %v", keysOf(w, named.TicketIDs()))
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
	if !slices.Equal(got.TicketIDs(), []int64{inQ1.ID}) {
		t.Fatalf("evidence: %v", keysOf(w, got.TicketIDs()))
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
	if len(got.TicketIDs()) != 0 {
		t.Fatalf("evidence below the floor: %v", keysOf(w, got.TicketIDs()))
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
	if !small.Exhaustive || len(small.TicketIDs()) != 2 || small.TicketIDs()[0] != recent.ID || !slices.Contains(small.TicketIDs(), old.ID) {
		t.Fatalf("small set: %v %v", small.Exhaustive, keysOf(w, small.TicketIDs()))
	}
	ranked := w.retrieve(w.admin, ask.Scope{}, q, hybrid)
	if ranked.Exhaustive || ranked.TicketIDs()[0] != recent.ID || !slices.Contains(ranked.TicketIDs(), payroll.ID) {
		t.Fatalf("ranked: %v", keysOf(w, ranked.TicketIDs()))
	}
}

// §11.3: with AI off, a ticket holding all the question's words ranks above
// tickets that repeat just one of them; tickets with one word still count.
func TestKeywordSearchPutsAllTheWordsFirst(t *testing.T) {
	w := newWorld(t)
	one := w.ticket("Overtime overtime overtime", &w.a, w.ot, "Overtime again: overtime rules for overtime.", "", "")
	both := w.ticket("Export for payroll", &w.a, w.ot, "Payroll imports the overtime hours.", "", "")
	found, err := ask.Retrieve(context.Background(), w.d.Pool, w.asker(w.admin), ask.Scope{}, "overtime payroll", nil, nil, hybrid)
	w.check(err)
	if len(found.TicketIDs()) != 2 || found.TicketIDs()[0] != both.ID || found.TicketIDs()[1] != one.ID {
		t.Fatalf("keyword order: %v", keysOf(w, found.TicketIDs()))
	}
}

// §11.4: over budget, the lowest-ranked ticket loses its comments first, then goes.
func TestPackKeepsTheBudget(t *testing.T) {
	w := newWorld(t)
	var items []ask.Evidence
	for _, title := range []string{"Overtime approval by HR", "Overtime export format", "Overtime cap"} {
		tk := w.ticket(title, &w.a, w.ot, "Supervisors are on leave.", "2026-05-01", "HR approves overtime.")
		_, err := w.d.Pool.Exec(context.Background(), `INSERT INTO comments (ticket_id, author_id, body) VALUES ($1, $2, $3)`,
			tk.ID, w.admin.ID, strings.Repeat("Discussed with the client at length. ", 8))
		w.check(err)
		src := must(indexer.Load(context.Background(), w.q, tk.ID))
		items = append(items, ask.Evidence{Ticket: &src})
	}
	text, keys, _ := ask.Pack(items, 10000, time.Time{})
	if len(keys) != 3 || !strings.Contains(text, "[HRIS-1] Change request · Client A · closed 2026-05-01 as Done · requested by Hana\nTitle: Overtime approval by HR\nAssigned to nobody · priority medium\nMenus: HR › Attendance › Overtime Approval\nReason: Supervisors are on leave.\nDecision (implemented): HR approves overtime.") ||
		strings.Count(text, "Comment ") != 3 {
		t.Fatalf("roomy budget:\n%s", text)
	}
	text, keys, _ = ask.Pack(items, 180, time.Time{}) // room for two tickets without comments
	if !slices.Equal(keys, []string{"HRIS-1", "HRIS-2"}) || strings.Count(text, "Comment ") != 0 {
		t.Fatalf("tight budget: %v\n%s", keys, text)
	}
}

// MSL-6: a ticket's block says which change it reverses and which reversed
// it, so the model can tell a change's reason from its reversal's. A link to
// another client's ticket stays out: a reader of this one may not see it.
func TestPackNamesReversals(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	old := w.ticket("Lower the approval threshold to Rp 25 juta", &w.a, w.ot, "An audit found unreviewed orders.", "2026-09-01", "Threshold lowered.")
	undo := w.ticket("Restore the approval threshold to Rp 50 juta", &w.a, w.ot, "Supervisors were overwhelmed.", "2026-09-20", "Threshold restored.")
	other := w.ticket("Client B's own threshold", &w.b, w.ot, "Client B asked.", "", "")
	for _, l := range []db.CreateLinkParams{{FromID: undo.ID, ToID: old.ID, Type: "reverses", CreatedBy: w.admin.ID}, {FromID: undo.ID, ToID: other.ID, Type: "related_to", CreatedBy: w.admin.ID}} {
		must(w.q.CreateLink(ctx, l))
	}
	var items []ask.Evidence
	for _, tk := range []db.Ticket{undo, old} {
		src := must(indexer.Load(ctx, w.q, tk.ID))
		items = append(items, ask.Evidence{Ticket: &src})
	}
	_, _, blocks := ask.Pack(items, 2500, time.Time{})
	if !strings.Contains(blocks[undo.Key], "Reverses "+old.Key+": Lower the approval threshold") ||
		!strings.Contains(blocks[old.Key], "Reversed later by "+undo.Key) || strings.Contains(blocks[undo.Key], other.Key) {
		t.Fatalf("blocks:\n%s\n\n%s", blocks[undo.Key], blocks[old.Key])
	}
}

// MSL-7: a ticket's block says who has it, its priority and, while open,
// whether it is overdue on the asker's today.
func TestPackSaysWhoHasItAndWhetherItIsLate(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	tk := w.ticket("Delivery note prints the wrong warehouse", &w.a, w.ot, "Drivers went to the wrong warehouse.", "", "")
	_, err := w.d.Pool.Exec(ctx, "UPDATE tickets SET assignee_id = $1, priority = 'urgent', due_date = '2026-09-26' WHERE id = $2", w.member.ID, tk.ID)
	w.check(err)
	src := must(indexer.Load(ctx, w.q, tk.ID))
	items := []ask.Evidence{{Ticket: &src}}
	for today, want := range map[string]string{
		"2026-09-29": "due 2026-09-26, overdue by 3 days\n", "2026-09-27": "due 2026-09-26, overdue by 1 day\n",
		"2026-09-26": "due 2026-09-26, due today\n", "2026-09-25": "due 2026-09-26, due tomorrow, not overdue\n",
		"2026-09-20": "due 2026-09-26, due in 6 days, not overdue\n", // MSL-43: a due date the model can't misread
	} {
		_, _, blocks := ask.Pack(items, 2500, must(time.Parse(time.DateOnly, today)))
		if b := blocks[tk.Key]; !strings.Contains(b, "Assigned to Rina · priority urgent · "+want) {
			t.Errorf("today %s:\n%s", today, b)
		}
	}
}

// §11.4: over budget, lower-ranked items give way before the best-ranked
// one is trimmed, so a meeting note keeps the follow-ups at the end of its body.
func TestPackKeepsTheBestItemsWhole(t *testing.T) {
	w := newWorld(t)
	body := "## Keputusan\n\n" + strings.Repeat("Periode payroll berjalan tanggal 21 sampai tanggal 20 bulan berjalan.\n", 10) +
		"\n## Tindak lanjut\n\n- Rahmat: kirim contoh file transfer Mandiri, paling lambat 3 Okt."
	note := w.note("Kickoff Payroll", &w.a, w.ot, "2026-09-29", body)
	src := must(indexer.LoadNote(context.Background(), w.q, note.ID))
	items := []ask.Evidence{{Note: &src}}
	for i := range 12 {
		tk := w.ticket(fmt.Sprintf("Overtime rule %d", i), &w.a, w.ot, "Supervisors are on leave.", "2026-05-01", "HR approves overtime.")
		tsrc := must(indexer.Load(context.Background(), w.q, tk.ID))
		items = append(items, ask.Evidence{Ticket: &tsrc})
	}
	text, keys, blocks := ask.Pack(items, 700, time.Time{})
	if keys[0] != note.Key || len(keys) == len(items) || !strings.Contains(text, "Rahmat: kirim contoh file transfer Mandiri") || blocks[note.Key] == "" {
		t.Fatalf("packed %v:\n%s", keys, text)
	}
}
