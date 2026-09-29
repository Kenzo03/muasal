package httpapi

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// GetWorkload counts each person's open tickets for the project's lead:
// every member who can own tickets, even with none, anyone else who still
// owns some (a former member, say), then unassigned work. The counts cover
// the tickets the caller may see (R-AC-2, R-AC-3).
func (s *Server) GetWorkload(w http.ResponseWriter, r *http.Request, key string, params GetWorkloadParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok || !staleDaysOK(w, params.StaleDays) {
		return
	}
	staleDays := int32(7)
	if params.StaleDays != nil {
		staleDays = *params.StaleDays
	}
	ctx := r.Context()
	counts, err := s.q.ProjectWorkload(ctx, db.ProjectWorkloadParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), StaleDays: staleDays,
		Today: s.today(pc.user),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	members, err := s.q.ListAssignees(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byUser := map[int64]WorkloadRow{}
	var unassigned *WorkloadRow
	for _, c := range counts {
		row := WorkloadRow{Open: int(c.Open), InProgress: int(c.InProgress), Overdue: int(c.Overdue), DueWeek: int(c.DueWeek),
			Stale: int(c.Stale), High: int(c.High)}
		if c.AssigneeID == nil {
			unassigned = &row
			continue
		}
		row.Assignee = &Ref{Id: *c.AssigneeID, Name: deref(c.AssigneeName)}
		byUser[*c.AssigneeID] = row
	}
	out := Workload{StaleDays: int(staleDays), Rows: []WorkloadRow{}}
	for _, m := range members {
		row, ok := byUser[m.ID]
		if !ok {
			row = WorkloadRow{Assignee: &Ref{Id: m.ID, Name: m.Name}}
		}
		delete(byUser, m.ID)
		out.Rows = append(out.Rows, row)
	}
	for _, row := range byUser { // owners who are no longer members
		out.Rows = append(out.Rows, row)
	}
	slices.SortStableFunc(out.Rows, func(a, b WorkloadRow) int {
		if c := strings.Compare(strings.ToLower(a.Assignee.Name), strings.ToLower(b.Assignee.Name)); c != 0 {
			return c
		}
		return cmp.Compare(a.Assignee.Id, b.Assignee.Id)
	})
	if unassigned != nil {
		out.Rows = append(out.Rows, *unassigned)
	}
	writeJSON(w, http.StatusOK, out)
}
