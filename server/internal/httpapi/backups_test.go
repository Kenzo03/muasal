package httpapi_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// FSD §15.6: the Backups page lists the dumps newest first with size and time,
// and "Run backup now" leaves a request for the backup service (§19.4).
func TestBackupsPage(t *testing.T) {
	dir := t.TempDir()
	e := newEnvWith(t, func(c *config.Config) { c.BackupsDir = dir })
	admin, _ := e.signedIn("admin@example.com", true)
	member, _ := e.signedIn("member@example.com", false)
	if err := os.Mkdir(filepath.Join(dir, "requests"), 0o777); err != nil { // backup.sh makes it at start
		t.Fatal(err)
	}
	for name, age := range map[string]time.Duration{"db-20260924-0100.dump": 48 * time.Hour, "db-20260925-0100.dump": 24 * time.Hour, "db-20260926-0100.dump.tmp": 0, "notes.txt": 0} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, 1024), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, time.Now().Add(-age), time.Now().Add(-age))
	}
	var list httpapi.BackupList
	if code := e.call(admin, http.MethodGet, "/admin/backups", nil, &list); code != http.StatusOK || len(list.Items) != 2 ||
		list.Items[0].File != "db-20260925-0100.dump" || list.Items[0].SizeBytes != 1024 || list.Requested || !list.SameDisk {
		t.Fatalf("list: %d %+v", code, list)
	}
	if code := e.call(admin, http.MethodPost, "/admin/backups/run", nil, nil); code != http.StatusAccepted {
		t.Fatalf("run: %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "requests", "run-now")); err != nil {
		t.Fatalf("no request for the backup service: %v", err)
	}
	if e.call(admin, http.MethodGet, "/admin/backups", nil, &list); !list.Requested {
		t.Fatal("the request should show as waiting")
	}
	for _, c := range []struct{ m, p string }{{http.MethodGet, "/admin/backups"}, {http.MethodPost, "/admin/backups/run"}} {
		if code := e.call(member, c.m, c.p, nil, nil); code != http.StatusForbidden {
			t.Fatalf("%s %s as a member: %d", c.m, c.p, code)
		}
	}
}

// Without the backups volume, the page says so instead of failing.
func TestBackupsWithoutTheService(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.BackupsDir = filepath.Join(t.TempDir(), "missing") })
	admin, _ := e.signedIn("admin@example.com", true)
	var list httpapi.BackupList
	if code := e.call(admin, http.MethodGet, "/admin/backups", nil, &list); code != http.StatusOK || len(list.Items) != 0 || list.SameDisk {
		t.Fatalf("list: %d %+v", code, list)
	}
	var p httpapi.Problem
	if code := e.call(admin, http.MethodPost, "/admin/backups/run", nil, &p); code != http.StatusServiceUnavailable || p.Code != "backup_unavailable" {
		t.Fatalf("run: %d %+v", code, p)
	}
}
