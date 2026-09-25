package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var aliasesError = FieldError{Field: "aliases", Code: "invalid", Message: "Use at most 20 aliases of at most 100 characters each"}

// clientAdmin lets system admins and project admins through: both list and
// create clients to link them to projects (FSD §5.1).
func (s *Server) clientAdmin(w http.ResponseWriter, r *http.Request) *db.User {
	u := s.requireUser(w, r)
	if u == nil {
		return nil
	}
	ok, err := access.AdminsAnything(r.Context(), s.q, u)
	if err != nil {
		s.fail(w, r, err)
		return nil
	}
	if !ok {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only admins can manage clients")
		return nil
	}
	return u
}

func (s *Server) ListClients(w http.ResponseWriter, r *http.Request) {
	if s.clientAdmin(w, r) == nil {
		return
	}
	rows, err := s.q.ListClients(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

func (s *Server) CreateClient(w http.ResponseWriter, r *http.Request) {
	u := s.clientAdmin(w, r)
	if u == nil {
		return
	}
	var in ClientCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	aliases, aliasesOK := cleanAliases(deref(in.Aliases))
	if fields := validateClient(&in.Name, in.Code, aliasesOK); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var c db.Client
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if c, err = q.CreateClient(ctx, db.CreateClientParams{Name: strings.TrimSpace(in.Name), Code: nonEmpty(in.Code), Aliases: aliases}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "client", c.ID, "create", clientAudit(c))
	})
	if isUniqueViolation(err) {
		clientNameTaken(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPIClient(c))
}

// UpdateClient renames or archives a client. Clients are shared by every
// project, so only system admins change them.
func (s *Server) UpdateClient(w http.ResponseWriter, r *http.Request, id int64) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var in ClientUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	var aliases []string
	aliasesOK := true
	if in.Aliases != nil {
		aliases, aliasesOK = cleanAliases(*in.Aliases)
	}
	if fields := validateClient(in.Name, in.Code, aliasesOK); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	ctx := r.Context()
	var updated db.Client
	err := s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		before, err := q.GetClient(ctx, id)
		if err != nil {
			return err
		}
		if updated, err = q.UpdateClient(ctx, db.UpdateClientParams{
			ID: id, Name: trimmed(in.Name), Code: trimmed(in.Code), Aliases: aliases, Archived: in.Archived,
		}); err != nil {
			return err
		}
		if updated.Name != before.Name { // chunks name the client (§13.1)
			ids, err := q.ListTicketIDsOfClient(ctx, id)
			if err != nil {
				return err
			}
			if err := s.index(ctx, tx, ids...); err != nil {
				return err
			}
		}
		return audit(ctx, q, webMeta(r), &admin.ID, "client", id, "update", changed(clientAudit(before), clientAudit(updated)))
	})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeProblem(w, http.StatusNotFound, "not_found", "Client not found")
	case isUniqueViolation(err):
		clientNameTaken(w)
	case err != nil:
		s.fail(w, r, err)
	default:
		writeJSON(w, http.StatusOK, toAPIClient(updated))
	}
}

func (s *Server) ListProjectClients(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	// Members pick from these on the ticket form, so they see their scope only (AC-TK-4).
	rows, err := s.q.ListProjectClients(r.Context(), db.ListProjectClientsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

// SetProjectClients replaces the project's client links (FSD §15.2). The
// database refuses to unlink a client that menus or member scopes still use.
func (s *Server) SetProjectClients(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in ProjectClientsUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.ClientIds == nil {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "required", Message: "List the project's clients"})
		return
	}
	ctx := r.Context()
	var rows []db.Client
	err := s.inTx(ctx, func(q *db.Queries) error {
		before, err := q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true})
		if err != nil {
			return err
		}
		if err := q.UnlinkClientsExcept(ctx, db.UnlinkClientsExceptParams{ProjectID: pc.project.ID, ClientIds: in.ClientIds}); err != nil {
			return err
		}
		if err := q.LinkClients(ctx, db.LinkClientsParams{ProjectID: pc.project.ID, ClientIds: in.ClientIds}); err != nil {
			return err
		}
		if rows, err = q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "set_clients",
			changed(map[string]any{"client_ids": idsOf(before)}, map[string]any{"client_ids": idsOf(rows)}))
	})
	switch constraintOf(err) {
	case "membership_clients_linked", "node_clients_linked", "tickets_client_linked":
		writeProblem(w, http.StatusConflict, "client_in_use", "A client you removed is still used by tickets, menus or member scopes in this project")
		return
	case "project_clients_client_fk":
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "client_ids", Code: "unknown_client", Message: "One of these clients does not exist"})
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ClientList{Items: toAPIClients(rows)})
}

func validateClient(name, code *string, aliasesOK bool) []FieldError {
	var f []FieldError
	if name != nil {
		if n := strings.TrimSpace(*name); n == "" || len(n) > 200 {
			f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
		}
	}
	if code != nil && len(strings.TrimSpace(*code)) > 20 {
		f = append(f, FieldError{Field: "code", Code: "invalid", Message: "Use at most 20 characters"})
	}
	if !aliasesOK {
		f = append(f, aliasesError)
	}
	return f
}

// cleanAliases trims aliases and drops empty ones; ok is false past 20
// aliases or 100 characters. The result is never nil, so an empty list clears.
func cleanAliases(in []string) (out []string, ok bool) {
	out = []string{}
	for _, a := range in {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		if len(a) > 100 {
			return nil, false
		}
		out = append(out, a)
	}
	return out, len(out) <= 20
}

// nonEmpty trims s and turns "" into nil.
func nonEmpty(s *string) *string {
	if s == nil {
		return nil
	}
	if t := strings.TrimSpace(*s); t != "" {
		return &t
	}
	return nil
}

func clientNameTaken(w http.ResponseWriter) {
	const msg = "A client with this name already exists"
	writeProblem(w, http.StatusConflict, "client_name_taken", msg, FieldError{Field: "name", Code: "client_name_taken", Message: msg})
}

func clientAudit(c db.Client) map[string]any {
	return map[string]any{"name": c.Name, "code": c.Code, "aliases": c.Aliases, "archived": c.ArchivedAt != nil}
}

func idsOf(clients []db.Client) []int64 {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return ids
}

func toAPIClient(c db.Client) Client {
	return Client{Id: c.ID, Name: c.Name, Code: c.Code, Aliases: orEmpty(c.Aliases), Archived: c.ArchivedAt != nil}
}

func toAPIClients(rows []db.Client) []Client {
	items := make([]Client, len(rows))
	for i, c := range rows {
		items[i] = toAPIClient(c)
	}
	return items
}
