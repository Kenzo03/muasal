package httpapi

import (
	"net/http"
	"strconv"
	"strings"

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
	limit, offset := 50, 0
	if params.Limit != nil {
		limit = min(max(int(*params.Limit), 1), 1000)
	}
	if params.Cursor != nil {
		n, err := strconv.Atoi(*params.Cursor)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_parameter", "The cursor is not one this API gave")
			return
		}
		offset = n
	}
	filter := db.ListTicketsParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		StatusID: params.StatusId, OpenOnly: deref(params.Open), ClientID: params.ClientId, CoreOnly: deref(params.Core),
		AssigneeID: params.AssigneeId, Q: strings.TrimSpace(deref(params.Q)), Sort: "updated",
		MissingReason: params.Missing != nil && *params.Missing == ListTicketsParamsMissingReason,
		MissingMenus:  params.Missing != nil && *params.Missing == ListTicketsParamsMissingMenus,
		Lim:           int32(limit + 1), Off: int32(offset),
	}
	if params.Category != nil {
		filter.Category = ptr(string(*params.Category))
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
		nodes, err := s.q.ListNodes(ctx, db.ListNodesParams{
			ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), IncludeArchived: true,
		})
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
		MissingReason: t.MissingReason, UpdatedAt: t.UpdatedAt,
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
