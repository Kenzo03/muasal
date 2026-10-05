package testdb_test

import (
	"fmt"
	"testing"

	"github.com/kenzo03/zettra/server/internal/testdb"
)

// go test runs packages in parallel against one server, and roles are
// server-wide, so New must not share a role between databases.
func TestNewIsSafeInParallel(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			testdb.New(t)
		})
	}
}
