package draft

import (
	"strings"
	"testing"

	"github.com/kenzo03/muasal/server/internal/docs"
)

// The outline sent with every part indents by level and stops at outlineChars.
func TestOutline(t *testing.T) {
	got := outline([]docs.Section{{Number: "s1", Title: "FSD", Level: 1}, {Number: "3.1.1", Title: "Dashboard", Level: 4}})
	if want := "[s1] FSD\n      [3.1.1] Dashboard\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	var many []docs.Section
	for range 200 {
		many = append(many, docs.Section{Number: "7.4", Title: "Overtime Approval", Level: 2})
	}
	if long := outline(many); len(long) > outlineChars+len("…\n") || !strings.HasSuffix(long, "…\n") {
		t.Fatalf("not cut: %d chars", len(long))
	}
}

// MSL-3: a node links to the heading that names it, whatever the model
// picked; the model's choice stands only when no heading names the node.
func TestLinkSection(t *testing.T) {
	part := []docs.Section{
		{Number: "s1", Title: "Arunika DMS — Spesifikasi Fungsional v1.0", Level: 1},
		{Number: "1", Title: "Penjualan", Level: 2},
		{Number: "1.1", Title: "Sales Order (SO-01)", Level: 3},
		{Number: "1.2", Title: "Persetujuan Sales Order (SO-02)", Level: 3},
	}
	for _, c := range []struct {
		path              []string
		model, want, code string
	}{
		{[]string{"Penjualan", "Persetujuan  sales order"}, "s1", "1.2", "SO-02"},
		{[]string{"Penjualan", "Sales Order (SO-01)"}, "s1", "1.1", "SO-01"},
		{[]string{"Penjualan"}, "s1", "1", ""},
		{[]string{"Penjualan", "Konsinyasi"}, "1.1", "1.1", ""},
	} {
		if got, code := linkSection(part, c.path, c.model); got != c.want || code != c.code {
			t.Errorf("%v: %q %q, want %q %q", c.path, got, code, c.want, c.code)
		}
	}
}
