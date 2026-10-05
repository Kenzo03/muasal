package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

func (s *Server) ListProjectMembers(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectMembers(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MemberList{Items: toAPIMembers(rows)})
}

// ListMemberCandidates lists who a project admin can pick as a member, so
// adding one is a pick, not a typed email (MSL-21). It shows only the people
// who already share a project with the admin, not the whole directory; a
// system admin sees every active user. Anyone else is added by exact email.
func (s *Server) ListMemberCandidates(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	users, err := s.q.ListMemberCandidates(r.Context(), db.ListMemberCandidatesParams{Everyone: pc.user.IsAdmin, UserID: pc.user.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := PersonList{Items: make([]Person, len(users))}
	for i, u := range users {
		out.Items[i] = Person{Id: u.ID, Name: u.Name, Email: u.Email}
	}
	writeJSON(w, http.StatusOK, out)
}

// SetProjectMembers replaces the project's members and their client scopes in
// one transaction, for one member or many at once (FSD §15.2).
func (s *Server) SetProjectMembers(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in MembersUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Members == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "members", Code: "required", Message: "List the project's members"})
		return
	}
	ctx := r.Context()
	members, fields, err := s.resolveMembers(ctx, in.Members)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A project admin cannot lock themselves out; system admins keep access anyway.
	if !pc.user.IsAdmin && !keepsAdmin(members, pc.user.ID) {
		fields = append(fields, FieldError{Field: "members", Code: "cannot_demote_self", Message: "You cannot remove your own project admin role"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	pid := pc.project.ID
	var rows []db.ListProjectMembersRow
	err = s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.ListProjectMembers(ctx, pid)
		if err != nil {
			return err
		}
		userIDs := make([]int64, len(members))
		for i, m := range members {
			userIDs[i] = m.userID
		}
		if err := q.DeleteMembershipsExcept(ctx, db.DeleteMembershipsExceptParams{ProjectID: pid, UserIds: userIDs}); err != nil {
			return err
		}
		for _, m := range members {
			if err := q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: m.userID, ProjectID: pid, Role: m.role, AllClients: m.all}); err != nil {
				return err
			}
			if err := q.ClearMembershipClients(ctx, db.ClearMembershipClientsParams{UserID: m.userID, ProjectID: pid}); err != nil {
				return err
			}
			if err := q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: m.userID, ProjectID: pid, ClientIds: orEmpty(m.clients)}); err != nil {
				return err
			}
		}
		if rows, err = q.ListProjectMembers(ctx, pid); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pid), &pc.user.ID, "project", pid, "set_members",
			changed(map[string]any{"members": memberAudit(before)}, map[string]any{"members": memberAudit(rows)}))
	})
	if constraintOf(err) == "membership_clients_linked" {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "members", Code: "client_not_linked", Message: "Scopes can list only the project's clients"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, MemberList{Items: toAPIMembers(rows)})
}

// member is one validated entry of a members update.
type member struct {
	userID  int64
	role    string
	all     bool
	clients []int64
}

// resolveMembers validates the entries and looks their users up by email.
func (s *Server) resolveMembers(ctx context.Context, in []MemberInput) ([]member, []FieldError, error) {
	var out []member
	var fields []FieldError
	seen := map[int64]bool{}
	for i, m := range in {
		at := fmt.Sprintf("members[%d].", i)
		if !m.Role.Valid() {
			fields = append(fields, FieldError{Field: at + "role", Code: "invalid", Message: "Choose admin, member or viewer"})
			continue
		}
		if m.Role == ProjectRoleAdmin && !m.AllClients {
			fields = append(fields, FieldError{Field: at + "all_clients", Code: "admin_needs_all_clients", Message: "Project admins always see all clients"})
			continue
		}
		u, err := s.q.GetUserByEmail(ctx, strings.TrimSpace(m.Email))
		if errors.Is(err, pgx.ErrNoRows) {
			fields = append(fields, FieldError{Field: at + "email", Code: "unknown_user", Message: "No user has this email"})
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if seen[u.ID] {
			fields = append(fields, FieldError{Field: at + "email", Code: "duplicate", Message: "This user is listed twice"})
			continue
		}
		seen[u.ID] = true
		entry := member{userID: u.ID, role: string(m.Role), all: m.AllClients}
		if !m.AllClients {
			entry.clients = deref(m.ClientIds)
		}
		out = append(out, entry)
	}
	return out, fields, nil
}

func keepsAdmin(members []member, userID int64) bool {
	for _, m := range members {
		if m.userID == userID && m.role == access.Admin {
			return true
		}
	}
	return false
}

func memberAudit(rows []db.ListProjectMembersRow) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, m := range rows {
		out[i] = map[string]any{"user_id": m.UserID, "role": m.Role, "all_clients": m.AllClients, "client_ids": orEmpty(m.ClientIds)}
	}
	return out
}

func toAPIMembers(rows []db.ListProjectMembersRow) []Member {
	items := make([]Member, len(rows))
	for i, m := range rows {
		items[i] = Member{
			UserId: m.UserID, Name: m.Name, Email: m.Email, Role: ProjectRole(m.Role),
			AllClients: m.AllClients, ClientIds: orEmpty(m.ClientIds),
		}
	}
	return items
}
