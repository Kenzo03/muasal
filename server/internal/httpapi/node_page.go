package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
)

// GetNode reads one node for its page (FSD §7.4), archived or not, with its
// parents from the top of the tree. A node the user may not see answers 404
// (R-AC-5), and it names only the clients in the user's scope (R-MR-8).
func (s *Server) GetNode(w http.ResponseWriter, r *http.Request, id int64) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	rows, err := s.visibleNodes(r.Context(), pc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	byID := map[int64]db.ListNodesRow{}
	for _, n := range rows {
		byID[n.ID] = n
	}
	out := NodeDetail{Node: toAPINodeRow(byID[id]), Path: []Ref{}, ProjectKey: pc.project.Key}
	for p := byID[id].ParentID; p != nil; p = byID[*p].ParentID {
		out.Path = append([]Ref{{Id: *p, Name: byID[*p].Name}}, out.Path...)
	}
	writeJSON(w, http.StatusOK, out)
}

// GetNodeTimeline lists the visible tickets on a node, by default with its
// sub-nodes (AC-MR-4): open ones first, then closed ones by close date, newest
// first, each with its decision record (FSD §7.4, AC-MR-3).
// ponytail: an offset cursor, like the ticket list.
func (s *Server) GetNodeTimeline(w http.ResponseWriter, r *http.Request, id int64, params GetNodeTimelineParams) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	limit, offset, ok := paging(w, params.Limit, params.Cursor)
	if !ok {
		return
	}
	ctx := r.Context()
	ids, err := s.nodeScope(ctx, pc, id, params.SubNodes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	filter := db.ListNodeTimelineParams{
		ProjectID: pc.project.ID, NodeIds: ids, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		ClientID: params.ClientId, CoreOnly: deref(params.Core), Lim: int32(limit + 1), Off: int32(offset),
	}
	if params.Type != nil {
		filter.Type = ptr(string(*params.Type))
	}
	if params.From != nil {
		filter.FromDate = &params.From.Time
	}
	if params.To != nil {
		filter.ToDate = &params.To.Time
	}
	rows, err := s.q.ListNodeTimeline(ctx, filter)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := TimelinePage{Items: make([]TimelineEntry, 0, len(rows))}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = ptr(strconv.Itoa(offset + limit))
	}
	for _, t := range rows {
		out.Items = append(out.Items, toTimelineEntry(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// GetNodeBehaviors lists the decisions in force on a node, by default with its
// sub-nodes: confirmed and implemented, core work first, then by client (FSD
// §7.4, story 5).
func (s *Server) GetNodeBehaviors(w http.ResponseWriter, r *http.Request, id int64, params GetNodeBehaviorsParams) {
	pc, _, ok := s.nodeFor(w, r, id, access.Viewer)
	if !ok {
		return
	}
	ctx := r.Context()
	ids, err := s.nodeScope(ctx, pc, id, params.SubNodes)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows, err := s.q.ListNodeBehaviors(ctx, db.ListNodeBehaviorsParams{
		ProjectID: pc.project.ID, NodeIds: ids, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := BehaviorList{Items: make([]Behavior, len(rows))}
	for i, b := range rows {
		out.Items[i] = Behavior{Key: b.Key, Title: b.Title, ClosedAt: b.ClosedAt, WhatChanged: b.WhatChanged, Why: b.Why, Alternatives: b.Alternatives}
		if b.ClientID != nil {
			out.Items[i].Client = &Ref{Id: *b.ClientID, Name: deref(b.ClientName)}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// visibleNodes lists the project's nodes the caller sees, archived ones too:
// node pages and sub-node filters keep their history (R-MR-4).
func (s *Server) visibleNodes(ctx context.Context, pc projectCtx) ([]db.ListNodesRow, error) {
	return s.q.ListNodes(ctx, db.ListNodesParams{
		ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs), IncludeArchived: true,
	})
}

// nodeScope is the node, plus its visible sub-nodes unless subNodes is false.
func (s *Server) nodeScope(ctx context.Context, pc projectCtx, id int64, subNodes *bool) ([]int64, error) {
	if subNodes != nil && !*subNodes {
		return []int64{id}, nil
	}
	rows, err := s.visibleNodes(ctx, pc)
	if err != nil {
		return nil, err
	}
	return subtree(rows, id), nil
}

func toTimelineEntry(t db.ListNodeTimelineRow) TimelineEntry {
	out := TimelineEntry{
		Key: t.Key, Title: t.Title, Type: TicketType(t.Type), Status: toAPIStatus(t.Status),
		CreatedAt: t.CreatedAt, ClosedAt: t.ClosedAt,
	}
	if t.ClientID != nil {
		out.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
	}
	if t.RequesterContactID != nil {
		out.Requester = TicketRequester{Kind: TicketRequesterKindContact, Id: *t.RequesterContactID,
			Name: deref(t.RequesterContactName), Title: t.RequesterContactTitle}
	} else {
		out.Requester = TicketRequester{Kind: TicketRequesterKindUser, Id: deref(t.RequesterUserID), Name: deref(t.RequesterUserName)}
	}
	if t.State != nil {
		out.Decision = &DecisionRecord{
			WhatChanged: deref(t.WhatChanged), Why: deref(t.Why), Alternatives: deref(t.Alternatives),
			Outcome: DecisionOutcome(deref(t.Outcome)), State: DecisionState(*t.State), ConfirmedAt: t.ConfirmedAt,
		}
		if t.ConfirmedBy != nil {
			out.Decision.ConfirmedBy = &Ref{Id: *t.ConfirmedBy, Name: deref(t.ConfirmerName)}
		}
	}
	return out
}
