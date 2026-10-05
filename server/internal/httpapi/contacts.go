package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/db"
)

// ListContacts searches the contacts the user may see (R-AC-6).
func (s *Server) ListContacts(w http.ResponseWriter, r *http.Request, params ListContactsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListContacts(r.Context(), db.ListContactsParams{
		IsAdmin: u.IsAdmin, UserID: u.ID, ClientID: params.ClientId, Internal: deref(params.Internal), Q: strings.TrimSpace(deref(params.Q)),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Contact, len(rows))
	for i, c := range rows {
		items[i] = toAPIContact(c)
	}
	writeJSON(w, http.StatusOK, ContactList{Items: items})
}

func (s *Server) CreateContact(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	var in ContactInput
	if !decodeJSON(w, r, &in) || !s.contactInputOK(w, r, u, in) {
		return
	}
	ctx := r.Context()
	var out Contact
	err := s.inTx(ctx, func(q *db.Queries) error {
		id, err := q.CreateContact(ctx, contactParams(in))
		if err != nil {
			return err
		}
		if out, err = readContact(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "contact", id, "create", contactAudit(out))
	})
	if constraintOf(err) == "contacts_client_fk" {
		contactClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) UpdateContact(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	rows, err := s.q.ListContacts(ctx, db.ListContactsParams{IsAdmin: u.IsAdmin, UserID: u.ID, ID: &id})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(rows) == 0 { // missing, or outside the user's scope (R-AC-7)
		writeProblem(w, http.StatusNotFound, "not_found", "Contact not found")
		return
	}
	may, err := s.mayEditContactsOf(ctx, u, rows[0].ClientID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if !may {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only members can edit contacts")
		return
	}
	var in ContactInput
	if !decodeJSON(w, r, &in) || !s.contactInputOK(w, r, u, in) {
		return
	}
	before := toAPIContact(rows[0])
	var out Contact
	err = s.inTx(ctx, func(q *db.Queries) error {
		p := contactParams(in)
		if err := q.UpdateContact(ctx, db.UpdateContactParams{
			ID: id, ClientID: p.ClientID, Name: p.Name, Title: p.Title, Email: p.Email, Phone: p.Phone,
		}); err != nil {
			return err
		}
		var err error
		if out, err = readContact(ctx, q, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r), &u.ID, "contact", id, "update", changed(contactAudit(before), contactAudit(out)))
	})
	if constraintOf(err) == "contacts_client_fk" {
		contactClientInvalid(w)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// contactInputOK validates a contact and checks that u may keep contacts of its
// client: members and project admins where the client is in scope, and for
// internal contacts, members of any project (FSD §5.1, §15.3).
func (s *Server) contactInputOK(w http.ResponseWriter, r *http.Request, u *db.User, in ContactInput) bool {
	if fields := validateContact(in); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return false
	}
	may, err := s.mayEditContactsOf(r.Context(), u, in.ClientId)
	switch {
	case err != nil:
		s.fail(w, r, err)
	case may:
		return true
	case in.ClientId != nil: // an out-of-scope client looks the same as a missing one
		contactClientInvalid(w)
	default:
		writeProblem(w, http.StatusForbidden, "forbidden", "Only members can add contacts")
	}
	return false
}

func (s *Server) mayEditContactsOf(ctx context.Context, u *db.User, clientID *int64) (bool, error) {
	if u.IsAdmin {
		return true, nil
	}
	return s.q.CanEditContactsOf(ctx, db.CanEditContactsOfParams{UserID: u.ID, ClientID: clientID})
}

// readContact reads a contact with its client name inside the write's transaction.
func readContact(ctx context.Context, q *db.Queries, id int64) (Contact, error) {
	rows, err := q.ListContacts(ctx, db.ListContactsParams{IsAdmin: true, ID: &id})
	if err != nil {
		return Contact{}, err
	}
	if len(rows) == 0 {
		return Contact{}, pgx.ErrNoRows
	}
	return toAPIContact(rows[0]), nil
}

func contactClientInvalid(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
		FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client you work with"})
}

func validateContact(in ContactInput) []FieldError {
	var f []FieldError
	if n := strings.TrimSpace(in.Name); n == "" || len(n) > 200 {
		f = append(f, FieldError{Field: "name", Code: "required", Message: "Enter a name of at most 200 characters"})
	}
	if t := nonEmpty(in.Title); t != nil && len(*t) > 200 {
		f = append(f, FieldError{Field: "title", Code: "invalid", Message: "Use at most 200 characters"})
	}
	if e := nonEmpty(in.Email); e != nil && !validEmail(*e) {
		f = append(f, FieldError{Field: "email", Code: "invalid", Message: "Enter a valid email address"})
	}
	if p := nonEmpty(in.Phone); p != nil && len(*p) > 50 {
		f = append(f, FieldError{Field: "phone", Code: "invalid", Message: "Use at most 50 characters"})
	}
	return f
}

func contactParams(in ContactInput) db.CreateContactParams {
	return db.CreateContactParams{
		ClientID: in.ClientId, Name: strings.TrimSpace(in.Name),
		Title: nonEmpty(in.Title), Email: nonEmpty(in.Email), Phone: nonEmpty(in.Phone),
	}
}

func contactAudit(c Contact) map[string]any {
	return map[string]any{"client_id": c.ClientId, "name": c.Name, "title": c.Title, "email": c.Email, "phone": c.Phone}
}

func toAPIContact(c db.ListContactsRow) Contact {
	return Contact{
		Id: c.ID, ClientId: c.ClientID, ClientName: c.ClientName, Name: c.Name,
		Title: c.Title, Email: c.Email, Phone: c.Phone,
	}
}
