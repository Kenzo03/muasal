package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

var colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func (s *Server) GetStatuses(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListStatuses(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, StatusList{Items: toAPIStatuses(rows)})
}

// SetStatuses replaces the project's ordered statuses (R-TK-1, R-TK-2). Tickets
// of a removed status move as move_to says, in the same transaction (R-TK-4).
func (s *Server) SetStatuses(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in StatusesUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	before, err := s.q.ListStatuses(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	inUse, err := s.q.ListStatusIDsInUse(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if fields := validateStatuses(in, before, inUse); len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var rows []db.Status
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.ClearDefaultStatus(ctx, pc.project.ID); err != nil {
			return err
		}
		keep := make([]int64, 0, len(in.Statuses))
		for i, st := range in.Statuses {
			name, def := strings.TrimSpace(st.Name), deref(st.IsDefault)
			if st.Id != nil {
				if _, err := q.UpdateStatus(ctx, db.UpdateStatusParams{
					ID: *st.Id, ProjectID: pc.project.ID, Name: name, Category: string(st.Category), Position: int32(i), Color: st.Color, IsDefault: def,
				}); err != nil {
					return err
				}
				keep = append(keep, *st.Id)
				continue
			}
			id, err := q.InsertStatus(ctx, db.InsertStatusParams{
				ProjectID: pc.project.ID, Name: name, Category: string(st.Category), Position: int32(i), Color: st.Color, IsDefault: def,
			})
			if err != nil {
				return err
			}
			keep = append(keep, id)
		}
		for _, m := range deref(in.MoveTo) {
			if err := q.MoveTicketsToStatus(ctx, db.MoveTicketsToStatusParams{ProjectID: pc.project.ID, FromID: m.From, ToID: m.To}); err != nil {
				return err
			}
		}
		if err := q.DeleteStatusesExcept(ctx, db.DeleteStatusesExceptParams{ProjectID: pc.project.ID, KeepIds: keep}); err != nil {
			return err
		}
		var err error
		if rows, err = q.ListStatuses(ctx, pc.project.ID); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "project", pc.project.ID, "set_statuses",
			changed(map[string]any{"statuses": statusAudit(before)}, map[string]any{"statuses": statusAudit(rows)}))
	})
	switch constraintOf(err) {
	case "tickets_status_same_project":
		writeProblem(w, http.StatusConflict, "status_in_use", "Tickets still use a status you removed; choose where they move")
		return
	case "statuses_name_uq":
		writeProblem(w, http.StatusConflict, "status_name_taken", "Two statuses have the same name")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, StatusList{Items: toAPIStatuses(rows)})
}

// validateStatuses checks R-TK-2 and the moves of R-TK-4 against the current
// list. A status that tickets use keeps its kind, so no ticket opens or closes
// without its close checks (R-TK-3): only To do and In progress swap.
func validateStatuses(in StatusesUpdate, before []db.Status, inUse []int64) []FieldError {
	var f []FieldError
	old := map[int64]db.Status{}
	for _, st := range before {
		old[st.ID] = st
	}
	names := map[string]bool{}
	kept := map[int64]StatusCategory{}
	defaults, done, cancelled := 0, 0, 0
	for i, st := range in.Statuses {
		at := fmt.Sprintf("statuses[%d].", i)
		name := strings.ToLower(strings.TrimSpace(st.Name))
		switch {
		case name == "" || len(name) > 50:
			f = append(f, FieldError{Field: at + "name", Code: "required", Message: "Enter a name of at most 50 characters"})
		case names[name]:
			f = append(f, FieldError{Field: at + "name", Code: "status_name_taken", Message: "Two statuses have this name"})
		}
		names[name] = true
		if !st.Category.Valid() {
			f = append(f, FieldError{Field: at + "category", Code: "invalid", Message: "Choose to do, in progress, done or cancelled"})
		}
		if !colorRe.MatchString(st.Color) {
			f = append(f, FieldError{Field: at + "color", Code: "invalid", Message: "Use a color like #2563EB"})
		}
		if st.Id != nil {
			o, ok := old[*st.Id]
			switch {
			case !ok:
				f = append(f, FieldError{Field: at + "id", Code: "invalid", Message: "Unknown status"})
			case o.Category != string(st.Category) && slices.Contains(inUse, *st.Id) &&
				(closedCategory(o.Category) || closedCategory(string(st.Category))):
				f = append(f, FieldError{Field: at + "category", Code: "status_category_in_use", Message: "Tickets use this status; move them before it opens or closes"})
			}
			kept[*st.Id] = st.Category
		}
		if deref(st.IsDefault) {
			defaults++
			if st.Category != StatusCategoryTodo {
				f = append(f, FieldError{Field: at + "is_default", Code: "invalid", Message: "The default status is a To do status"})
			}
		}
		switch st.Category {
		case StatusCategoryDone:
			done++
		case StatusCategoryCancelled:
			cancelled++
		}
	}
	if defaults != 1 {
		f = append(f, FieldError{Field: "statuses", Code: "invalid", Message: "Choose exactly one default status"})
	}
	if done == 0 || cancelled == 0 {
		f = append(f, FieldError{Field: "statuses", Code: "invalid", Message: "Keep at least one Done and one Cancelled status"})
	}
	for i, m := range deref(in.MoveTo) {
		from, existed := old[m.From]
		_, stays := kept[m.From]
		to, isKept := kept[m.To]
		// Open tickets land in any open status; closed ones only in the same
		// category, so their decisions keep their outcome (R-DC-2).
		sameKind := from.Category == string(to) || (!closedCategory(from.Category) && !closedCategory(string(to)))
		if !existed || stays || !isKept || !sameKind {
			f = append(f, FieldError{Field: fmt.Sprintf("move_to[%d]", i), Code: "invalid", Message: "Move tickets from a removed status to a kept one of the same kind"})
		}
	}
	return f
}

// closedCategory reports whether a status category closes tickets (R-TK-3).
func closedCategory(c string) bool {
	return c == string(StatusCategoryDone) || c == string(StatusCategoryCancelled)
}

// ListAssignees lists who can own the project's tickets.
func (s *Server) ListAssignees(w http.ResponseWriter, r *http.Request, key string, params ListAssigneesParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListAssignees(r.Context(), db.ListAssigneesParams{ProjectID: pc.project.ID, ClientID: params.ClientId})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]Ref, len(rows))
	for i, u := range rows {
		items[i] = Ref{Id: u.ID, Name: u.Name}
	}
	writeJSON(w, http.StatusOK, RefList{Items: items})
}

func statusAudit(rows []db.Status) []map[string]any {
	out := make([]map[string]any, len(rows))
	for i, st := range rows {
		out[i] = map[string]any{"name": st.Name, "category": st.Category, "color": st.Color, "default": st.IsDefault}
	}
	return out
}

func toAPIStatus(st db.Status) Status {
	return Status{Id: st.ID, Name: st.Name, Category: StatusCategory(st.Category), Color: st.Color, Position: st.Position, IsDefault: st.IsDefault}
}

func toAPIStatuses(rows []db.Status) []Status {
	items := make([]Status, len(rows))
	for i, st := range rows {
		items[i] = toAPIStatus(st)
	}
	return items
}
