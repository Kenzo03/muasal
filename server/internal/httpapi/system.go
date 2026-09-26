package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/llm"
)

// systemFigures is what System status and /metrics show (FSD §18.3).
type systemFigures struct {
	dbBytes int64
	jobs    map[string]int64
	asks    map[string]int64
	mode    string
	model   *bool // nil when AI is off
	modelEr string
	disks   []DiskUse
}

var askStatuses = []string{"answered", "not_enough_info", "ai_off", "error"}

func (s *Server) figures(ctx context.Context, probe bool) (systemFigures, error) {
	f := systemFigures{jobs: map[string]int64{}, asks: map[string]int64{}}
	if err := s.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&f.dbBytes); err != nil {
		return f, err
	}
	rows, err := s.pool.Query(ctx, "SELECT state::text, count(*) FROM river_job GROUP BY state")
	if err != nil {
		return f, err
	}
	for rows.Next() {
		var state string
		var n int64
		if err := rows.Scan(&state, &n); err != nil {
			return f, err
		}
		f.jobs[state] = n
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	for _, st := range askStatuses {
		f.asks[st] = 0
	}
	rows, err = s.pool.Query(ctx, "SELECT status, count(*) FROM ask_queries GROUP BY status")
	if err != nil {
		return f, err
	}
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			return f, err
		}
		f.asks[st] = n
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	cur, err := s.ai.Store.Get(ctx)
	if err != nil {
		return f, err
	}
	f.mode = string(cur.Mode)
	if probe && cur.Mode != ai.ModeOff {
		ok := true
		if err := s.probeModel(ctx, cur); err != nil {
			ok, f.modelEr = false, err.Error()
		}
		f.model = &ok
	}
	f.disks = []DiskUse{diskUse("attachments", s.cfg.AttachmentsDir), diskUse("backups", s.cfg.BackupsDir)}
	return f, nil
}

// probeModel lists the models of both endpoints, within 3 seconds.
func (s *Server) probeModel(ctx context.Context, cur ai.Settings) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for _, client := range []func(ai.Settings) (*llm.Client, error){s.ai.ChatClient, s.ai.EmbedClient} {
		c, err := client(cur)
		if err != nil {
			return err
		}
		if _, err := c.Models(ctx); err != nil {
			return err
		}
	}
	return nil
}

func diskUse(volume, path string) DiskUse {
	d := DiskUse{Volume: DiskUseVolume(volume), Path: path}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			d.Missing = ptr(true)
		}
		return d
	}
	bs := int64(st.Bsize)
	d.TotalBytes = int64(st.Blocks) * bs
	d.UsedBytes = (int64(st.Blocks) - int64(st.Bfree)) * bs
	return d
}

// GetSystemStatus is Admin → System status (§18.3).
func (s *Server) GetSystemStatus(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	f, err := s.figures(r.Context(), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := SystemStatus{DatabaseBytes: f.dbBytes, Jobs: f.jobs, Disks: f.disks, Warnings: []SystemStatusWarnings{}}
	out.Model.Mode = AIMode(f.mode)
	out.Model.Reachable = f.model
	if f.modelEr != "" {
		out.Model.Error = &f.modelEr
	}
	for _, d := range f.disks {
		if d.TotalBytes > 0 && d.UsedBytes*10 >= d.TotalBytes*8 {
			out.Warnings = append(out.Warnings, SystemStatusWarnings("disk_"+string(d.Volume)))
		}
	}
	if f.model != nil && !*f.model {
		out.Warnings = append(out.Warnings, SystemStatusWarningsModelUnreachable)
	}
	if f.jobs["discarded"] > 0 {
		out.Warnings = append(out.Warnings, SystemStatusWarningsJobsFailed)
	}
	writeJSON(w, http.StatusOK, out)
}

// metrics serves the figures in Prometheus text (§18.3). Caddy routes only
// /api and /webhooks to the app, so this stays on the internal network.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	f, err := s.figures(r.Context(), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var b strings.Builder
	gauge := func(name, help string) { fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name) }
	gauge("muasal_database_bytes", "Size of the database.")
	fmt.Fprintf(&b, "muasal_database_bytes %d\n", f.dbBytes)
	gauge("muasal_jobs", "Background jobs by state.")
	states := make([]string, 0, len(f.jobs))
	for st := range f.jobs {
		states = append(states, st)
	}
	slices.Sort(states)
	for _, st := range states {
		fmt.Fprintf(&b, "muasal_jobs{state=%q} %d\n", st, f.jobs[st])
	}
	gauge("muasal_ask_queries", "Questions in the Ask log by status.")
	for _, st := range askStatuses {
		fmt.Fprintf(&b, "muasal_ask_queries{status=%q} %d\n", st, f.asks[st])
	}
	gauge("muasal_disk_used_bytes", "Used bytes on the volume's filesystem.")
	for _, d := range f.disks {
		fmt.Fprintf(&b, "muasal_disk_used_bytes{volume=%q} %d\n", d.Volume, d.UsedBytes)
	}
	gauge("muasal_disk_total_bytes", "Size of the volume's filesystem.")
	for _, d := range f.disks {
		fmt.Fprintf(&b, "muasal_disk_total_bytes{volume=%q} %d\n", d.Volume, d.TotalBytes)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
