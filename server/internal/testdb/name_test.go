package testdb

import (
	"regexp"
	"testing"
)

// Packages test in parallel against one server, and the clock can repeat
// (microseconds on macOS), so two New calls must never pick the same name. The
// name is also an unquoted identifier, so it must stay lowercase.
func TestNewNameNeverRepeats(t *testing.T) {
	identifier := regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)
	seen := map[string]bool{}
	for range 10000 {
		n := newName()
		if !identifier.MatchString(n) {
			t.Fatalf("%q is not a lowercase identifier", n)
		}
		if seen[n] {
			t.Fatalf("%q repeated", n)
		}
		seen[n] = true
	}
}
