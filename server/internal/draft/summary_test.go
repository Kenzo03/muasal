package draft

import (
	"fmt"
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

func TestTitle(t *testing.T) {
	from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)
	if got := Title("Payroll", "Client B", from, to, "en"); got != "Payroll changes for Client B, 1 Jan 2026 – 23 Aug 2026" {
		t.Fatal(got)
	}
	if got := Title("Payroll", "", from, to, "id"); got != "Perubahan Payroll untuk semua klien, 1 Jan 2026 – 23 Agu 2026" {
		t.Fatal(got)
	}
}
