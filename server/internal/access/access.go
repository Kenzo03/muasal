// Package access decides what a user may see and do in a project (FSD §5).
package access

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Project roles, lowest first (FSD §5.1).
const (
	Viewer = "viewer"
	Member = "member"
	Admin  = "admin"
)

var rank = map[string]int{Viewer: 1, Member: 2, Admin: 3}

// Scope is one user's standing in one project: a role and the clients they may
// see there (R-AC-1). ClientIDs matters only when AllClients is false.
type Scope struct {
	Role       string
	AllClients bool
	ClientIDs  []int64
}

// Allows reports whether the scope's role is at least need.
func (s Scope) Allows(need string) bool { return rank[s.Role] >= rank[need] }

// ForProject reads u's scope in a project fresh from the database, so a changed
// scope applies on the user's next request (R-AC-8). System admins act as
// project admins with all clients everywhere; member is false when u does not
// belong to the project.
func ForProject(ctx context.Context, q *db.Queries, u *db.User, projectID int64) (scope Scope, member bool, err error) {
	if u.IsAdmin {
		return Scope{Role: Admin, AllClients: true}, true, nil
	}
	m, err := q.GetMembership(ctx, db.GetMembershipParams{UserID: u.ID, ProjectID: projectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Scope{}, false, nil
	}
	if err != nil {
		return Scope{}, false, err
	}
	return Scope{Role: m.Role, AllClients: m.AllClients, ClientIDs: m.ClientIds}, true, nil
}

// AdminsAnything reports whether u is a system admin or a project admin
// somewhere; both may list and create clients (FSD §5.1).
func AdminsAnything(ctx context.Context, q *db.Queries, u *db.User) (bool, error) {
	if u.IsAdmin {
		return true, nil
	}
	return q.IsProjectAdminAnywhere(ctx, u.ID)
}
