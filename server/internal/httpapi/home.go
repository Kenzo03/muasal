package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
)

// ListMyTickets serves Home's My tickets (FSD §6.4): the open tickets assigned
// to the caller in any project, as far as the caller may see them (R-AC-7).
// The counts cover every view and project, whatever view the page shows.
func (s *Server) ListMyTickets(w http.ResponseWriter, r *http.Request, params ListMyTicketsParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	view := MyTicketsViewAll
	if params.View != nil {
		view = *params.View
	}
	switch view {
	case MyTicketsViewAll, MyTicketsViewOverdue, MyTicketsViewWeek, MyTicketsViewIncomplete:
	default:
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "view is one of all, overdue, week and incomplete")
		return
	}
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
	ctx := r.Context()
	today := s.today(u)
	rows, err := s.q.ListMyTickets(ctx, db.ListMyTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID, View: string(view), Today: today, Lim: int32(limit + 1), Off: int32(offset)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	counts, err := s.q.CountMyTickets(ctx, db.CountMyTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID, Today: today})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	projects, err := s.q.CountMyTicketsByProject(ctx, db.CountMyTicketsByProjectParams{IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := MyTicketsPage{
		Items:    make([]MyTicket, 0, len(rows)),
		Counts:   MyTicketsCounts{All: int(counts.AllOpen), Overdue: int(counts.Overdue), Week: int(counts.Week), Incomplete: int(counts.Incomplete)},
		Projects: make([]ProjectCount, len(projects)),
	}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		it := MyTicket{
			Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Priority: Priority(t.Priority), Status: toAPIStatus(t.Status),
			MissingReason: t.MissingReason, MissingMenus: t.MissingMenus,
		}
		if t.Menu != "" {
			it.Menu = &t.Menu
		}
		if t.DueDate != nil {
			it.DueDate = &openapi_types.Date{Time: *t.DueDate}
		}
		if t.ClientID != nil {
			it.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
		}
		out.Items = append(out.Items, it)
	}
	for i, p := range projects {
		out.Projects[i] = ProjectCount{Key: p.Key, Open: int(p.Open)}
	}
	writeJSON(w, http.StatusOK, out)
}

// ListMyAttention serves Home's Needs attention (MSL-9): a project admin with
// nothing assigned still sees what in their projects is late, unowned or
// incomplete, counted from today on their calendar.
func (s *Server) ListMyAttention(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	rows, err := s.q.ListAttention(r.Context(), db.ListAttentionParams{IsAdmin: u.IsAdmin, UserID: u.ID, Today: s.today(u)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := AttentionList{Items: []ProjectAttention{}}
	for _, p := range rows {
		a := ProjectAttention{Key: p.Key, Name: p.Name, Overdue: int(p.Overdue), Week: int(p.Week), Unassigned: int(p.Unassigned),
			NoReason: int(p.NoReason), WeakReason: int(p.WeakReason), NoMenu: int(p.NoMenu), Stale: int(p.Stale)}
		if a.Overdue+a.Week+a.Unassigned+a.NoReason+a.WeakReason+a.NoMenu+a.Stale > 0 {
			out.Items = append(out.Items, a)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// GetSetupStatus tells a system admin how far the server is set up, for
// Home's first-run checklist (MSL-18).
func (s *Server) GetSetupStatus(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	if !u.IsAdmin {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only system admins see this")
		return
	}
	ctx := r.Context()
	st, err := s.q.SetupStatus(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	settings, err := s.ai.Store.Get(ctx)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ai": settings.Mode != ai.ModeOff, "invited": st.Invited, "project": st.Project,
		"tree": st.Tree, "history": st.History, "first_project": st.FirstProject})
}

// GetProjectSetup tells a project admin how far the project is set up, for the
// checklist on its board (MSL-48).
func (s *Server) GetProjectSetup(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	st, err := s.q.ProjectSetupStatus(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, ProjectSetup{Clients: st.Clients, Team: st.Team, Documents: st.Documents, Tree: st.Tree, Tickets: st.Tickets, Repos: st.Repos})
}

// ListMyUpdates serves Home's Recently updated (FSD §6.4): the tickets the
// caller may see that changed last, each with its latest change. A comment
// change names its author but carries no text.
func (s *Server) ListMyUpdates(w http.ResponseWriter, r *http.Request) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	tickets, err := s.q.ListRecentTickets(ctx, db.ListRecentTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]int64, len(tickets))
	for i, t := range tickets {
		ids[i] = t.ID
	}
	changes, err := s.q.LatestTicketChanges(ctx, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	latest := map[int64]ActivityItem{}
	for _, c := range changes {
		item := ActivityItem{Kind: ActivityItemKindComment, At: c.At}
		if c.Action != "comment" {
			var fields map[string]any
			if err := json.Unmarshal(c.Changes, &fields); err != nil {
				s.fail(w, r, err)
				return
			}
			item.Kind, item.Action, item.Changes = ActivityItemKindEvent, ptr(c.Action), &fields
		}
		if c.ActorID != 0 {
			item.Actor = &Ref{Id: c.ActorID, Name: deref(c.ActorName)}
		}
		latest[c.TicketID] = item
	}
	out := RecentTicketList{Items: make([]RecentTicket, len(tickets))}
	for i, t := range tickets {
		out.Items[i] = RecentTicket{Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Status: toAPIStatus(t.Status), UpdatedAt: t.UpdatedAt}
		if c, ok := latest[t.ID]; ok {
			out.Items[i].Change = &c
		}
	}
	writeJSON(w, http.StatusOK, out)
}
