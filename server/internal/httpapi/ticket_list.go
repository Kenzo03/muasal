package httpapi

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// ListTickets serves the list and the board (FSD §8.4, §8.5): one project's
// visible tickets, filtered and sorted in SQL.
// ponytail: an offset cursor; switch to keyset pages if deep pages get slow.
func (s *Server) ListTickets(w http.ResponseWriter, r *http.Request, key string, params ListTicketsParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
	csvOut := params.Format != nil && *params.Format == ListTicketsParamsFormatCsv
	if csvOut {
		limit, offset = exportMax, 0
	}
	if !staleDaysOK(w, params.StaleDays) {
		return
	}
	filter := db.ListTicketsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		StatusID: params.StatusId, OpenOnly: deref(params.Open), ClientID: params.ClientId, CoreOnly: deref(params.Core),
		AssigneeID: params.AssigneeId, Unassigned: deref(params.Unassigned), Q: strings.TrimSpace(deref(params.Q)), Sort: "updated",
		MissingReason: params.Missing != nil && *params.Missing == ListTicketsParamsMissingReason,
		MissingMenus:  params.Missing != nil && *params.Missing == ListTicketsParamsMissingMenus,
		WeakReason:    params.Missing != nil && *params.Missing == ListTicketsParamsMissingWeakReason,
		ClosedDays:    params.ClosedDays, StaleDays: params.StaleDays, Today: s.today(pc.user),
		Label: lowerPtr(params.Label),
		Lim:   int32(limit + 1), Off: int32(offset),
	}
	if params.Category != nil {
		filter.Category = ptr(string(*params.Category))
	}
	if params.Due != nil {
		filter.Due = ptr(string(*params.Due))
	}
	if params.Type != nil {
		filter.Type = ptr(string(*params.Type))
	}
	if params.Sort != nil {
		filter.Sort = string(*params.Sort)
	}
	if deref(params.Mine) {
		filter.AssigneeID = &pc.user.ID
	}
	ctx := r.Context()
	if params.NodeId != nil {
		nodes, err := s.visibleNodes(ctx, pc)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		filter.NodeIds = subtree(nodes, *params.NodeId) // the node and its sub-nodes (AC-MR-4)
	}
	rows, err := s.q.ListTickets(ctx, filter)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if csvOut {
		s.writeTicketsCSV(w, r, pc, rows[:min(len(rows), limit)])
		return
	}
	page := TicketPage{Items: make([]TicketSummary, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		page.Items = append(page.Items, toTicketSummary(t))
	}
	writeJSON(w, http.StatusOK, page)
}

// staleDaysOK answers 400 for a stale_days outside 1–365.
func staleDaysOK(w http.ResponseWriter, days *int32) bool {
	if days != nil && (*days < 1 || *days > 365) {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "stale_days is a number of days from 1 to 365")
		return false
	}
	return true
}

// paging reads a page size (50 by default, at most 1,000) and an offset cursor
// that an earlier page gave; a cursor it did not give answers 400.
func paging(w http.ResponseWriter, lim *int32, cursor *string) (int, int, bool) {
	limit, offset := 50, 0
	if lim != nil {
		limit = min(max(int(*lim), 1), 1000)
	}
	if cursor != nil {
		n, err := strconv.Atoi(*cursor)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", "The cursor is not one this API gave")
			return 0, 0, false
		}
		offset = n
	}
	return limit, offset, true
}

// subtree lists root and every node below it; an unknown root gives an empty,
// non-nil list, which matches no ticket.
func subtree(rows []db.ListNodesRow, root int64) []int64 {
	kids := map[int64][]int64{}
	found := false
	for _, n := range rows {
		found = found || n.ID == root
		if n.ParentID != nil {
			kids[*n.ParentID] = append(kids[*n.ParentID], n.ID)
		}
	}
	out := []int64{}
	if !found {
		return out
	}
	for stack := []int64{root}; len(stack) > 0; {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, id)
		stack = append(stack, kids[id]...)
	}
	return out
}

func toTicketSummary(t db.ListTicketsRow) TicketSummary {
	out := TicketSummary{
		Id: t.ID, Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Priority: Priority(t.Priority),
		StatusId: t.StatusID, RequesterName: t.RequesterName, NodeNames: orEmpty(t.NodeNames),
		MissingReason: t.MissingReason, UpdatedAt: t.UpdatedAt, Labels: &t.Labels,
	}
	if t.ChecklistTotal > 0 {
		out.Checklist = &struct {
			Done  int `json:"done"`
			Total int `json:"total"`
		}{int(t.ChecklistDone), int(t.ChecklistTotal)}
	}
	if t.DueDate != nil {
		out.DueDate = &openapi_types.Date{Time: *t.DueDate}
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
	}
	if t.AssigneeID != nil {
		out.Assignee = &Ref{Id: *t.AssigneeID, Name: deref(t.AssigneeName)}
	}
	return out
}

// exportMax bounds one CSV export; a larger filter is cut there.
const exportMax = 10000

// writeTicketsCSV sends the filtered tickets as a spreadsheet-friendly CSV
// (FSD §8.5): UTF-8 with a byte-order mark, one row per ticket, the list's
// columns plus the dates.
func (s *Server) writeTicketsCSV(w http.ResponseWriter, r *http.Request, pc projectCtx, rows []db.ListTicketsRow) {
	statuses, err := s.q.ListStatuses(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	statusName := map[int64]string{}
	for _, st := range statuses {
		statusName[st.ID] = st.Name
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-tickets-%s.csv"`, pc.project.Key, s.now().Format("2006-01-02")))
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"key", "title", "type", "status", "client", "assignee", "requested_by", "menus", "priority", "updated", "due", "missing_reason"})
	for _, t := range rows {
		due := ""
		if t.DueDate != nil {
			due = t.DueDate.Format(time.DateOnly)
		}
		_ = cw.Write([]string{
			t.Key, csvSafe(t.Title), t.Type, statusName[t.StatusID], csvSafe(deref(t.ClientName)), csvSafe(deref(t.AssigneeName)),
			csvSafe(t.RequesterName), csvSafe(strings.Join(t.NodeNames, "; ")), t.Priority, t.UpdatedAt.UTC().Format(time.RFC3339), due,
			strconv.FormatBool(t.MissingReason),
		})
	}
	cw.Flush()
}

// csvSafe keeps a spreadsheet from running a cell as a formula (CSV injection).
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// ListProjectLabels lists the labels a project's tickets use, most used first (MSL-56).
func (s *Server) ListProjectLabels(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.q.ListProjectLabels(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type item struct {
		Label string `json:"label"`
		Uses  int    `json:"uses"`
	}
	out := struct {
		Items []item `json:"items"`
	}{Items: make([]item, len(rows))}
	for i, r := range rows {
		out.Items[i] = item{r.Label, int(r.Uses)}
	}
	writeJSON(w, http.StatusOK, out)
}

func lowerPtr(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	l := strings.ToLower(strings.Join(strings.Fields(*s), " "))
	return &l
}
