package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var projectKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}$`)

// projectCtx is a request's project and the caller's standing in it.
type projectCtx struct {
	user    *db.User
	project db.Project
	scope   access.Scope
	// ownRole is the caller's role before an archive made the project
	// read-only (MSL-64); it decides who restores it.
	ownRole string
}

// projectFor resolves {key} for the signed-in user. A missing project and one
// the user does not belong to both answer 404, so projects never reveal
// themselves (R-AC-7); a role below need answers 403.
func (s *Server) projectFor(w http.ResponseWriter, r *http.Request, key, need string) (projectCtx, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return projectCtx{}, false
	}
	p, err := s.q.GetProjectByKey(r.Context(), key)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Project not found")
		return projectCtx{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, false
	}
	pc, ok := s.memberOf(w, r, u, p)
	if ok && !pc.scope.Allows(need) {
		denyRole(w, pc)
		return projectCtx{}, false
	}
	return pc, ok
}

// denyRole answers a role below what a request needs: 409 in an archived
// project, which takes no changes until it is restored (MSL-64), else 403.
func denyRole(w http.ResponseWriter, pc projectCtx) {
	if pc.project.ArchivedAt != nil {
		writeProblem(w, http.StatusConflict, "project_archived", "This project is archived; a project admin can restore it")
		return
	}
	writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
}

// memberOf reads u's scope in p and answers 404 when u is not a member.
func (s *Server) memberOf(w http.ResponseWriter, r *http.Request, u *db.User, p db.Project) (projectCtx, bool) {
	scope, member, err := access.ForProject(r.Context(), s.q, u, p.ID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, false
	}
	if !member {
		writeProblem(w, http.StatusNotFound, "not_found", "Project not found")
		return projectCtx{}, false
	}
	pc := projectCtx{user: u, project: p, scope: scope, ownRole: scope.Role}
	if p.ArchivedAt != nil {
		pc.scope.Role = access.Viewer // every write checks the role (MSL-64)
	}
	return pc, true
}

func (s *Server) ListProjects(w http.ResponseWriter, r *http.Request, params ListProjectsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListProjects(r.Context(), db.ListProjectsParams{UserID: u.ID, IsAdmin: u.IsAdmin, Archived: deref(params.Archived)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Project, len(rows))
	for i, row := range rows {
		role := access.Admin
		if !u.IsAdmin {
			role = *row.Role
		}
		items[i] = toAPIProject(row.Project, role)
	}
	writeJSON(w, http.StatusOK, ProjectList{Items: items})
}

func (s *Server) CreateProject(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in ProjectCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	key := strings.ToUpper(strings.TrimSpace(in.Key))
	fields := validateProject(&key, &in.Name, in.Description)
	ctx := r.Context()
	var template *db.Project
	if k := strings.ToUpper(strings.TrimSpace(deref(in.TemplateKey))); k != "" {
		t, err := s.q.GetProjectByKey(ctx, k)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			fields = append(fields, FieldError{Field: "template_key", Code: "invalid", Message: "Choose an existing project to copy"})
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			template = &t
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var p db.Project
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if p, err = q.CreateProject(ctx, db.CreateProjectParams{
			Key: key, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(deref(in.Description)),
		}); err != nil {
			return err
		}
		// MSL-47: whoever creates a project runs it until they hand it over.
		if err := q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: admin.ID, ProjectID: p.ID, Role: "admin", AllClients: true}); err != nil {
			return err
		}
		change := projectAudit(p)
		if template != nil {
			if err := copyTemplate(ctx, q, template.ID, p.ID); err != nil {
				return err
			}
			change["template"] = template.Key
		}
		return audit(ctx, q, webMeta(r).inProject(p.ID), &admin.ID, "project", p.ID, "create", change)
	})
	if isUniqueViolation(err) {
		projectKeyTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIProject(p, access.Admin))
}

func (s *Server) GetProject(w http.ResponseWriter, r *http.Request, key string) {
	if pc, ok := s.projectFor(w, r, key, access.Viewer); ok {
		writeJSON(w, http.StatusOK, toAPIProject(pc.project, pc.ownRole))
	}
}

// ArchiveProject makes a finished project read-only and takes it out of
// pickers and Home; RestoreProject brings it back (MSL-64). Both need the
// caller's own admin role, which the archive itself turns read-only.
func (s *Server) ArchiveProject(w http.ResponseWriter, r *http.Request, key string) {
	s.setArchived(w, r, key, true)
}

func (s *Server) RestoreProject(w http.ResponseWriter, r *http.Request, key string) {
	s.setArchived(w, r, key, false)
}

func (s *Server) setArchived(w http.ResponseWriter, r *http.Request, key string, archived bool) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	if pc.ownRole != access.Admin {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return
	}
	ctx := r.Context()
	var updated db.Project
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.SetProjectArchived(ctx, db.SetProjectArchivedParams{ID: pc.project.ID, Archived: archived}); err != nil {
			return err
		}
		action := "restore"
		if archived {
			action = "archive"
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, action, nil)
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIProject(updated, pc.ownRole))
}

func (s *Server) UpdateProject(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in ProjectUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Key != nil {
		in.Key = ptr(strings.ToUpper(strings.TrimSpace(*in.Key)))
	}
	if fields := validateProject(in.Key, in.Name, in.Description); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	if in.Key != nil && *in.Key != pc.project.Key && pc.project.TicketSeq > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "key", Code: "project_key_fixed", Message: "The key cannot change once the project has tickets"})
		return
	}
	ctx := r.Context()
	var updated db.Project
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if updated, err = q.UpdateProject(ctx, db.UpdateProjectParams{
			ID: pc.project.ID, Key: in.Key, Name: trimmed(in.Name), Description: trimmed(in.Description),
		}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "update",
			changed(projectAudit(pc.project), projectAudit(updated)))
	})
	if isUniqueViolation(err) {
		projectKeyTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPIProject(updated, pc.scope.Role))
}

func validateProject(key, name, description *string) []FieldError {
	var f []FieldError
	if key != nil && !projectKeyRe.MatchString(*key) {
		f = append(f, FieldError{Field: "key", Code: "invalid", Message: "Use 2 to 10 capital letters or digits, starting with a letter"})
	}
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if description != nil && len(*description) > 2000 {
		f = append(f, FieldError{Field: "description", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	return f
}

func projectKeyTaken(w http.ResponseWriter) {
	const msg = "Another project already uses this key"
	writeProblem(w, http.StatusConflict, "project_key_taken", msg, FieldError{Field: "key", Code: "project_key_taken", Message: msg})
}

func projectAudit(p db.Project) map[string]any {
	return map[string]any{"key": p.Key, "name": p.Name, "description": p.Description}
}

// copyTemplate gives a new project a template's statuses, in place of the
// defaults, and its live module tree in the same order. Client-specific menus
// arrive shared: the new project links no clients yet.
func copyTemplate(ctx context.Context, q *db.Queries, from, to int64) error {
	if err := q.DeleteProjectStatuses(ctx, to); err != nil {
		return err
	}
	if err := q.CopyStatuses(ctx, db.CopyStatusesParams{ProjectID: to, TemplateID: from}); err != nil {
		return err
	}
	nodes, err := q.ListNodes(ctx, db.ListNodesParams{ProjectID: from, AllClients: true, ClientIds: []int64{}})
	if err != nil {
		return err
	}
	kids := map[int64][]db.ListNodesRow{} // by parent id, 0 for the top; siblings in position order
	for _, n := range nodes {
		kids[deref(n.ParentID)] = append(kids[deref(n.ParentID)], n)
	}
	var copyUnder func(parent int64, newParent *int64) error
	copyUnder = func(parent int64, newParent *int64) error {
		for _, n := range kids[parent] {
			c, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: to, ParentID: newParent, Type: n.Type, Name: n.Name,
				Code: n.Code, Aliases: n.Aliases, Description: n.Description})
			if err != nil {
				return err
			}
			if err := copyUnder(n.ID, &c.ID); err != nil {
				return err
			}
		}
		return nil
	}
	return copyUnder(0, nil)
}

// toAPIProject shows the caller's own role, as viewer while the project is
// archived (MSL-64).
func toAPIProject(p db.Project, role string) Project {
	out := Project{Id: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Role: ProjectRole(role), CreatedAt: p.CreatedAt,
		ArchivedAt: p.ArchivedAt, CanRestore: ptr(role == access.Admin)}
	if p.ArchivedAt != nil {
		out.Role = ProjectRole(access.Viewer)
	}
	return out
}
