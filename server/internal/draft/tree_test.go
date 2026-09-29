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
