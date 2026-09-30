package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// ListReleases lists a project's releases (MSL-67).
func (s *Server) ListReleases(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListReleases(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := ReleaseList{Items: make([]Release, len(rows))}
	for i, rl := range rows {
		out.Items[i] = toAPIRelease(rl)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateRelease adds a release such as v1.0 to a project.
func (s *Server) CreateRelease(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in ReleaseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	name, ok := releaseName(w, in)
	if !ok {
		return
	}
	ctx := r.Context()
	var rl db.Release
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if rl, err = q.CreateRelease(ctx, db.CreateReleaseParams{ProjectID: pc.project.ID, Name: name, ReleasedOn: dateOf(in.ReleasedOn)}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "release", rl.ID, "create", map[string]any{"name": name})
	})
	s.releaseSaved(w, r, rl, err, http.StatusCreated)
}

// UpdateRelease renames a release or sets the day it shipped.
func (s *Server) UpdateRelease(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rl, err := s.q.GetRelease(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Release not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.q.GetProjectByID(ctx, rl.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return
	}
	if !pc.scope.Allows(access.Member) {
		denyRole(w, pc)
		return
	}
	var in ReleaseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	name, ok := releaseName(w, in)
	if !ok {
		return
	}
	var updated db.Release
	err = s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateRelease(ctx, db.UpdateReleaseParams{ID: id, Name: name, ReleasedOn: dateOf(in.ReleasedOn)}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(p.ID), &u.ID, "release", id, "update", changed(
			map[string]any{"name": rl.Name, "released_on": rl.ReleasedOn}, map[string]any{"name": updated.Name, "released_on": updated.ReleasedOn}))
	})
	s.releaseSaved(w, r, updated, err, http.StatusOK)
}

func releaseName(w http.ResponseWriter, in ReleaseInput) (string, bool) {
	name := strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 50 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "name", Code: "invalid", Message: "Use 1 to 50 characters"})
		return "", false
	}
	return name, true
}

func (s *Server) releaseSaved(w http.ResponseWriter, r *http.Request, rl db.Release, err error, status int) {
	switch {
	case isUniqueViolation(err):
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "name", Code: "taken", Message: "This project already has a release with this name"})
	case err != nil:
		s.fail(w, r, err)
	default:
		writeJSON(w, status, toAPIRelease(rl))
	}
}

func toAPIRelease(rl db.Release) Release {
	out := Release{Id: rl.ID, Name: rl.Name}
	if rl.ReleasedOn != nil {
		out.ReleasedOn = &openapi_types.Date{Time: *rl.ReleasedOn}
	}
	return out
}

// dateOf is an optional API date as the database takes it.
func dateOf(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}
	return &d.Time
}
