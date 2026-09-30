package ask_test

import (
	"slices"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
)

func ptr[T any](v T) *T { return &v }

var jakarta, _ = time.LoadLocation("Asia/Jakarta")

// now is 23 Sep 2026, 10:00 in Jakarta.
var now = time.Date(2026, 9, 23, 10, 0, 0, 0, jakarta)

func catalog() ask.Catalog {
	return ask.Catalog{
		Clients: []db.ListScopeClientsRow{
			{ID: 4, Name: "Client A", Code: ptr("CLA"), Aliases: []string{"Arunika"}},
			{ID: 5, Name: "Bumi Logistik", Aliases: []string{}},
		},
		Nodes: []db.ListScopeNodesRow{
			{ID: 1, Name: "HR", Aliases: []string{}},
			{ID: 2, ParentID: ptr[int64](1), Name: "Attendance", Aliases: []string{"Absensi"}},
			{ID: 3, ParentID: ptr[int64](2), Name: "Overtime Approval", Code: ptr("OT-APR"), Aliases: []string{"approval lembur"}},
			{ID: 9, Name: "Payroll", Aliases: []string{"Penggajian"}},
		},
		People: []db.ListScopePeopleRow{
			{Kind: "contact", ID: 7, Name: "Budi Santoso"},
			{Kind: "user", ID: 8, Name: "Indah Permata"},
		},
	}
}

func day(s string) *time.Time {
	d, _ := time.ParseInLocation(time.DateOnly, s, jakarta)
	return &d
}

func sameDay(a, b *time.Time) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Equal(*b))
}

// AC-AK-1 and §11.2: clients by name, code, alias or a close spelling; the
// deepest node; people only after a cue word; ticket keys.
func TestDetectNamesClientsNodesPeopleAndKeys(t *testing.T) {
	for _, c := range []struct {
		q                       string
		clients, nodes, us, cts []int64
		keys                    []string
	}{
		{q: "Kenapa approval lembur skip supervisor untuk Client A?", clients: []int64{4}, nodes: []int64{3}},
		{q: "What changed in Payroll for Arunika?", clients: []int64{4}, nodes: []int64{9}},
		{q: "Why does Bumi Logistic skip the step?", clients: []int64{5}},                             // the English spelling
		{q: "Attendance and Overtime Approval rules for CLA", clients: []int64{4}, nodes: []int64{3}}, // Overtime is under Attendance
		{q: "Apa yang diminta oleh Budi di HR?", nodes: []int64{1}, cts: []int64{7}},
		{q: "Is Indah still the rule for payroll?", nodes: []int64{9}}, // no cue word: not a person
		{q: "Tickets requested by Indah Permata", us: []int64{8}},
		{q: "What did hris-231 and HRIS-240 change?", keys: []string{"HRIS-231", "HRIS-240"}},
		// MSL-38: a question about other or every client keeps them all in scope.
		{q: "Berapa toleransi selisih stok untuk Client A dan untuk klien lain?"},
		{q: "Is Bumi Logistik's tolerance the same as for other clients?"},
		{q: "Apa bedanya Arunika dengan semua klien?"},
		{q: "Which clients other than CLA skip the step?"},
	} {
		d := ask.Detect(catalog(), c.q, now)
		if !slices.Equal(d.ClientIDs, c.clients) || !slices.Equal(d.NodeIDs, c.nodes) || !slices.Equal(d.UserIDs, c.us) ||
			!slices.Equal(d.ContactIDs, c.cts) || !slices.Equal(d.Keys, c.keys) {
			t.Errorf("%q: %+v", c.q, d)
		}
	}
}

// §11.2: date phrases in both languages; a month without a year is its latest past occurrence.
func TestDetectDatePhrases(t *testing.T) {
	for _, c := range []struct {
		q        string
		from, to *time.Time
	}{
		{"What changed since January?", day("2026-01-01"), nil},
		{"Apa yang berubah sejak Oktober?", day("2025-10-01"), nil}, // October 2026 has not come yet
		{"Tickets in 2025", day("2025-01-01"), day("2025-12-31")},
		{"Perubahan tahun 2025", day("2025-01-01"), day("2025-12-31")},
		{"What changed last month?", day("2026-08-01"), day("2026-08-31")},
		{"Apa saja bulan lalu?", day("2026-08-01"), day("2026-08-31")},
		{"Changes this year", day("2026-01-01"), day("2026-09-23")},
		{"Keputusan tahun ini", day("2026-01-01"), day("2026-09-23")},
		{"What shipped in Q1 2026?", day("2026-01-01"), day("2026-03-31")},
		{"Between March and May", day("2026-03-01"), day("2026-05-31")},
		{"antara Maret dan Mei 2026", day("2026-03-01"), day("2026-05-31")},
		{"Changes since 2026-02-15", day("2026-02-15"), nil},
		{"May I see the overtime rules?", nil, nil}, // the verb, not the month
		{"What did HRIS-2025 change?", nil, nil},    // a key, not a year
	} {
		d := ask.Detect(catalog(), c.q, now)
		if !sameDay(d.From, c.from) || !sameDay(d.To, c.to) {
			t.Errorf("%q: from %v to %v", c.q, d.From, d.To)
		}
	}
}

// §10.5: the question's language, from common words; a tie uses the UI language.
func TestLanguage(t *testing.T) {
	for _, c := range []struct{ q, ui, want string }{
		{"Kenapa approval lembur skip supervisor untuk Client A?", "en", "id"},
		{"Why does overtime approval skip the supervisor for Client A?", "id", "en"},
		{"HRIS-231?", "en", "en"},
		{"HRIS-231?", "id", "id"},
	} {
		if got := ask.Language(c.q, c.ui); got != c.want {
			t.Errorf("%q (%s): %s", c.q, c.ui, got)
		}
	}
}

// §10.2: a detected chip shows a name, so Detect labels what it found. A node
// shows its path, as in the menu picker.
func TestDetectLabelsItsChips(t *testing.T) {
	d := ask.Detect(catalog(), "Kenapa approval lembur diminta oleh Budi untuk Client A?", now)
	want := []ask.Label{
		{Kind: "client", ID: 4, Label: "Client A"},
		{Kind: "node", ID: 3, Label: "HR › Attendance › Overtime Approval"},
		{Kind: "contact", ID: 7, Label: "Budi Santoso"},
	}
	if !slices.Equal(d.Labels, want) {
		t.Fatalf("labels: %+v, want %+v", d.Labels, want)
	}
}

// MSL-38: "And for the other clients?" drops the earlier client instead of
// carrying it over, and keeps the menu.
func TestCarryOverStopsAtOtherClients(t *testing.T) {
	prev := ask.Detect(catalog(), "Why does overtime approval skip the supervisor for Client A?", now)
	next := ask.CarryOver(prev, ask.Detect(catalog(), "And for the other clients?", now))
	if next.ClientIDs != nil || !next.AllClients || !slices.Equal(next.NodeIDs, []int64{3}) ||
		slices.ContainsFunc(next.Labels, func(l ask.Label) bool { return l.Kind == "client" }) {
		t.Fatalf("%+v", next)
	}
}
