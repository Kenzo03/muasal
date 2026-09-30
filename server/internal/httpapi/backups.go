package httpapi

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
)

// dumpName is what backup.sh writes: db-YYYYMMDD-HHMM.dump (FSD §19.4).
var dumpName = regexp.MustCompile(`^db-\d{8}-\d{4}\.dump$`)

// runNow is where "Run backup now" leaves its request. backup.sh makes the
// folder writable for the app, which runs as a non-root user.
func (s *Server) runNow() string { return filepath.Join(s.cfg.BackupsDir, "requests", "run-now") }

// ListBackups lists the database dumps, newest first (FSD §15.6).
func (s *Server) ListBackups(w http.ResponseWriter, r *http.Request) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	out := BackupList{Location: s.cfg.BackupsDir, Items: []Backup{}}
	entries, err := os.ReadDir(s.cfg.BackupsDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.fail(w, r, err)
		return
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !dumpName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // removed by rotation meanwhile
		}
		out.Items = append(out.Items, Backup{File: e.Name(), SizeBytes: info.Size(), CreatedAt: info.ModTime()})
	}
	slices.SortFunc(out.Items, func(a, b Backup) int { return strings.Compare(b.File, a.File) })
	_, err = os.Stat(s.runNow())
	out.Requested = err == nil
	out.SameDisk = sameDisk(s.cfg.BackupsDir, s.cfg.AttachmentsDir)
	writeJSON(w, http.StatusOK, out)
}

// sameDisk reports whether two folders are on one filesystem (MSL-40). Docker
// keeps named volumes on the host's disk, so a default install says yes until
// the backups volume is mounted elsewhere, such as on a NAS.
func sameDisk(a, b string) bool {
	x, errA := os.Stat(a)
	y, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	sx, okX := x.Sys().(*syscall.Stat_t)
	sy, okY := y.Sys().(*syscall.Stat_t)
	return okX && okY && sx.Dev == sy.Dev
}

// RunBackup asks the backup service for a backup now (§19.4).
func (s *Server) RunBackup(w http.ResponseWriter, r *http.Request) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	if err := os.WriteFile(s.runNow(), nil, 0o644); err != nil {
		s.log.Warn("backup request failed", "request_id", requestIDFrom(r.Context()), "err", err)
		writeProblem(w, http.StatusServiceUnavailable, "backup_unavailable", "The backup service is not set up on this server")
		return
	}
	if err := audit(r.Context(), s.q, webMeta(r), &u.ID, "backup", 0, "run", nil); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
