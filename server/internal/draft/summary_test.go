package draft

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// §12.1: up to 40 items take one call; more split per menu group of up to 40.
func TestBatches(t *testing.T) {
	var items []Item
	for i := range 40 {
		items = append(items, Item{Key: fmt.Sprint("A-", i), Menu: "Overtime"})
	}
	if b := batches(items); len(b) != 1 {
		t.Fatalf("40 items: %d calls", len(b))
	}
	for i := range 45 {
		items = append(items, Item{Key: fmt.Sprint("B-", i), Menu: "Payroll"})
	}
	b := batches(items)
	if len(b) != 3 || len(b[0]) != 40 || b[1][0].Menu != "Payroll" || len(b[1]) != 40 || len(b[2]) != 5 {
		t.Fatalf("85 items: %d calls", len(b))
	}
}

// MSL-1: a ticked item no bullet cites still gets one, from its decision
// record, and a reversed item's bullet says what reversed it.
func TestCoverAddsWhatTheModelLeftOut(t *testing.T) {
	day := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{Key: "DMS-1", Menu: "Persetujuan", Date: day, Change: "Ambang turun ke Rp 25 juta.", Why: "Audit internal.", ReversedBy: "DMS-10"},
		{Key: "DMS-10", Menu: "Persetujuan", Date: day, Change: "Ambang kembali ke Rp 50 juta.", Why: "Supervisor kewalahan."},
		{Key: "DMS-DN1", Menu: "Sales Order", Date: day, Title: "Rapat bulanan MJD"},
	}
	sections := map[string]*Section{"Persetujuan": {Menu: "Persetujuan"}, "Sales Order": {Menu: "Sales Order"}}
	sections["Persetujuan"].Bullets = []Bullet{{Date: day, Text: "Ambang turun ke Rp 25 juta.", Keys: []string{"DMS-1"}}}
	cover(sections, items, map[string]bool{"DMS-1": true}, "id")
	got := sections["Persetujuan"].Bullets
	if len(got) != 2 || got[1].Keys[0] != "DMS-10" || got[1].Text != "Ambang kembali ke Rp 50 juta." || got[1].Why != "Supervisor kewalahan." {
		t.Fatalf("Persetujuan: %+v", got)
	}
	if note := sections["Sales Order"].Bullets; len(note) != 1 || note[0].Text != "Rapat bulanan MJD" {
		t.Fatalf("Sales Order: %+v", note)
	}
	fresh := map[string]*Section{"Persetujuan": {Menu: "Persetujuan"}, "Sales Order": {Menu: "Sales Order"}}
	cover(fresh, items, map[string]bool{"DMS-10": true, "DMS-DN1": true}, "id")
	if b := fresh["Persetujuan"].Bullets; len(b) != 1 || b[0].Text != "Ambang turun ke Rp 25 juta. Kemudian dibalik oleh DMS-10." {
		t.Fatalf("reversed item: %+v", b)
	}
	text := itemsText(items, map[string]string{"DMS-10": "DMS-1"})
	if !strings.Contains(text, "[DMS-1] ") || !strings.Contains(text, "reversed later by DMS-10, no longer in force") || !strings.Contains(text, "— reverses DMS-1") {
		t.Fatalf("items text:\n%s", text)
	}
}

func TestTitle(t *testing.T) {
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	if got := Title("Payroll", "Client B", from, to, "en"); got != "Payroll changes for Client B, 1 Jan 2026 – 23 Aug 2026" {
		t.Fatal(got)
	}
	if got := Title("Payroll", "", from, to, "id"); got != "Perubahan Payroll untuk semua klien, 1 Jan 2026 – 23 Agu 2026" {
		t.Fatal(got)
	}
}
