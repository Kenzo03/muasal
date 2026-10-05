package httpapi_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/kenzo03/zettra/server/internal/config"
	"github.com/kenzo03/zettra/server/internal/httpapi"
	"github.com/kenzo03/zettra/server/internal/llm/llmtest"
)

// FSD §18.3: Admin → System status shows the database size, the job queue,
// the model server's health and the disk use of both volumes.
func TestSystemStatus(t *testing.T) {
	backups := t.TempDir()
	e := newEnvWith(t, func(c *config.Config) { c.BackupsDir = backups })
	admin, _ := e.signedIn("admin@example.com", true)
	member, _ := e.signedIn("member@example.com", false)
	var st httpapi.SystemStatus
	if code := e.call(admin, http.MethodGet, "/admin/system/status", nil, &st); code != http.StatusOK {
		t.Fatalf("status: %d", code)
	}
	if st.DatabaseBytes <= 0 || st.Model.Mode != httpapi.AIModeOff || st.Model.Reachable != nil || len(st.Disks) != 2 ||
		st.Disks[0].Volume != "attachments" || st.Disks[1].Volume != "backups" || st.Disks[1].TotalBytes <= 0 {
		t.Fatalf("status: %+v", st)
	}
	// Disk warnings follow the host's real disk use, from 80%; nothing else warns.
	for _, d := range st.Disks {
		full := d.UsedBytes*10 >= d.TotalBytes*8
		if warned := contains(st.Warnings, httpapi.SystemStatusWarnings("disk_"+string(d.Volume))); warned != full {
			t.Errorf("%s at %d of %d bytes: warned %v", d.Volume, d.UsedBytes, d.TotalBytes, warned)
		}
	}
	for _, w := range st.Warnings {
		if !strings.HasPrefix(string(w), "disk_") {
			t.Errorf("unexpected warning %q", w)
		}
	}
	if code := e.call(member, http.MethodGet, "/admin/system/status", nil, nil); code != http.StatusForbidden {
		t.Fatalf("as a member: %d", code)
	}

	// With AI on and the model server down, the page warns.
	fake := e.localAI(admin)
	fake.Set(func(s *llmtest.Server) { s.Down = true })
	if e.call(admin, http.MethodGet, "/admin/system/status", nil, &st); st.Model.Reachable == nil || *st.Model.Reachable || !contains(st.Warnings, "model_unreachable") {
		t.Fatalf("model down: %+v %v", st.Model, st.Warnings)
	}
	// The status names the failure, never the model server's own answer.
	fake.Set(func(s *llmtest.Server) { s.DownBody = "SECRET" })
	if e.call(admin, http.MethodGet, "/admin/system/status", nil, &st); st.Model.Error == nil || strings.Contains(*st.Model.Error, "SECRET") {
		t.Fatalf("model error: %v", st.Model.Error)
	}
}

// §18.3: /metrics serves the same figures in Prometheus text for customers
// who scrape them. Caddy does not route it, so it stays internal.
func TestMetrics(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.url + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{"# TYPE zettra_database_bytes gauge", "zettra_disk_total_bytes{volume=\"attachments\"}", "zettra_ask_queries{status=\"answered\"} 0"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Errorf("content type %q", resp.Header.Get("Content-Type"))
	}
}

func contains[T comparable](xs []T, x T) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
