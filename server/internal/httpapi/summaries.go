package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/draft"
)

// summaryItem is one item in a summary's scope, with its ids for the model input.
type summaryItem struct {
	api      SummaryItem
	ticketID int64 // or
	noteID   int64
	decision *db.ListNodeTimelineRow
	noteBody string
}

// summaryScope resolves a builder's scope: every visible closed ticket and
// decision note on the node and its sub-nodes, by menu, oldest first (§12.1).
func (s *Server) summaryScope(w http.ResponseWriter, r *http.Request, in SummaryScope) (projectCtx, db.Node, string, []summaryItem, bool) {
	pc, ok := s.projectFor(w, r, in.ProjectKey, access.Member)
	if !ok {
		return projectCtx{}, db.Node{}, "", nil, false
	}
	var fields []FieldError
	if in.To.Before(in.From.Time) {
		fields = append(fields, FieldError{Field: "to", Code: "invalid", Message: "End on or after the start date"})
	}
	if in.ClientId != nil && !pc.scope.Sees(in.ClientId) {
		fields = append(fields, FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client in your scope"})
	}
	ctx := r.Context()
	nodes, err := s.visibleNodes(ctx, pc)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, "", nil, false
	}
	names := map[int64]string{}
	for _, n := range nodes {
		names[n.ID] = n.Name
	}
	if _, ok := names[in.NodeId]; !ok && in.NodeId != 0 { // 0 is the whole project (MSL-16)
		fields = append(fields, FieldError{Field: "node_id", Code: "invalid", Message: "Choose a menu or module of this project"})
	}
	var release db.Release // MSL-67: one release's tickets
	if in.ReleaseId != nil {
		release, err = s.q.GetRelease(ctx, *in.ReleaseId)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			s.fail(w, r, err)
			return projectCtx{}, db.Node{}, "", nil, false
		}
		if err != nil || release.ProjectID != pc.project.ID {
			fields = append(fields, FieldError{Field: "release_id", Code: "invalid", Message: "Choose a release of this project"})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return projectCtx{}, db.Node{}, "", nil, false
	}
	// The whole project, as a weekly summary covers it, reads by the project's name.
	node, ids := db.Node{Name: pc.project.Name}, make([]int64, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	if in.NodeId != 0 {
		if node, err = s.q.GetNode(ctx, in.NodeId); err != nil {
			s.fail(w, r, err)
			return projectCtx{}, db.Node{}, "", nil, false
		}
		ids = subtree(nodes, in.NodeId)
	}
	if in.ReleaseId != nil {
		node.Name += " " + release.Name // "HRIS v1.0 changes for …"
	}
	clientName := ""
	if in.ClientId != nil {
		c, err := s.q.GetClient(ctx, *in.ClientId)
		if err != nil {
			s.fail(w, r, err)
			return projectCtx{}, db.Node{}, "", nil, false
		}
		clientName = c.Name
	}
	items, err := s.summaryItems(ctx, pc, ids, in.NodeId == 0, in.ClientId, in.ReleaseId, in.From.Time, in.To.Time, deref(in.IncludeCancelled), names)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, db.Node{}, "", nil, false
	}
	return pc, node, clientName, items, true
}

// summaryItems lists the visible closed tickets and decision notes on the nodes
// ids between from and to, for one client with core work (clientID) or all;
// whole says ids are the whole project, which holds notes without a menu. A
// release takes only its tickets, and no notes (MSL-67).
func (s *Server) summaryItems(ctx context.Context, pc projectCtx, ids []int64, whole bool, clientID, releaseID *int64, from, to time.Time, cancelled bool, names map[int64]string) ([]summaryItem, error) {
	rows, err := s.q.ListNodeTimeline(ctx, db.ListNodeTimelineParams{
		ProjectID: pc.project.ID, NodeIds: ids, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		FromDate: &from, ToDate: &to, Lim: 2000,
	})
	if err != nil {
		return nil, err
	}
	notes, err := s.q.ListNodeNotes(ctx, db.ListNodeNotesParams{
		ProjectID: pc.project.ID, NodeIds: ids, Whole: whole, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
		FromDate: &from, ToDate: &to,
	})
	if err != nil {
		return nil, err
	}
	// One client's summary also covers core work and notes for all clients.
	forClient := func(id *int64) bool { return clientID == nil || id == nil || *id == *clientID }
	var ticketIDs, noteIDs []int64
	var items []summaryItem
	for i := range rows {
		t := &rows[i]
		if !forClient(t.ClientID) || releaseID != nil && (t.ReleaseID == nil || *t.ReleaseID != *releaseID) {
			continue
		}
		isCancelled := t.Status.Category == string(StatusCategoryCancelled)
		if t.ClosedAt == nil || !(t.Status.Category == string(StatusCategoryDone) || isCancelled && cancelled) {
			continue
		}
		it := summaryItem{ticketID: t.ID, decision: t, api: SummaryItem{
			Key: t.Key, Kind: SummaryItemKindTicket, Title: t.Title, Date: openapi_types.Date{Time: *t.ClosedAt},
			Client: t.ClientName, RequestedBy: ptr(deref(t.RequesterContactName) + deref(t.RequesterUserName)),
		}}
		if isCancelled {
			it.api.Cancelled = ptr(true)
		}
		if t.AcceptedOn != nil { // MSL-66
			it.api.AcceptedBy, it.api.AcceptedOn = t.AcceptedContactName, &openapi_types.Date{Time: *t.AcceptedOn}
		}
		items = append(items, it)
		ticketIDs = append(ticketIDs, t.ID)
	}
	for _, n := range notes {
		if !forClient(n.ClientID) || releaseID != nil {
			continue
		}
		items = append(items, summaryItem{noteID: n.ID, noteBody: n.Body, api: SummaryItem{
			Key: n.Key, Kind: SummaryItemKindNote, Title: n.Title, Date: openapi_types.Date{Time: n.DecidedOn}, Client: n.ClientName,
		}})
		noteIDs = append(noteIDs, n.ID)
	}
	// Each item sits under its first menu inside the scope.
	inScope := map[int64]bool{}
	for _, id := range ids {
		inScope[id] = true
	}
	menuOf := map[string]string{}
	tn, err := s.q.ListNodesOfTickets(ctx, ticketIDs)
	if err != nil {
		return nil, err
	}
	for _, x := range tn {
		if k := fmt.Sprint("t", x.TicketID); menuOf[k] == "" && inScope[x.NodeID] {
			menuOf[k] = names[x.NodeID]
		}
	}
	nn, err := s.q.ListNodesOfNotes(ctx, noteIDs)
	if err != nil {
		return nil, err
	}
	for _, x := range nn {
		if k := fmt.Sprint("n", x.NoteID); menuOf[k] == "" && inScope[x.NodeID] {
			menuOf[k] = names[x.NodeID]
		}
	}
	for i := range items {
		if items[i].ticketID != 0 {
			items[i].api.Menu = menuOf[fmt.Sprint("t", items[i].ticketID)]
		} else {
			// A note without a menu is about the whole project (MSL-59).
			items[i].api.Menu = cmp.Or(menuOf[fmt.Sprint("n", items[i].noteID)], pc.project.Name)
		}
	}
	slices.SortStableFunc(items, func(a, b summaryItem) int {
		if c := strings.Compare(strings.ToLower(a.api.Menu), strings.ToLower(b.api.Menu)); c != 0 {
			return c
		}
		if c := a.api.Date.Compare(b.api.Date.Time); c != 0 {
			return c
		}
		return strings.Compare(a.api.Key, b.api.Key)
	})
	return items, nil
}

// PreviewSummary lists the items a summary would cover; no model call.
func (s *Server) PreviewSummary(w http.ResponseWriter, r *http.Request) {
	var in SummaryScope
	if !decodeJSON(w, r, &in) {
		return
	}
	_, _, _, items, ok := s.summaryScope(w, r, in)
	if !ok {
		return
	}
	st, err := s.ai.Store.Get(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := SummaryPreview{Items: make([]SummaryItem, len(items)), Cloud: st.Mode == ai.ModeBYOK}
	if st.Mode != ai.ModeOff {
		out.Model = st.Badge()
	}
	for i, it := range items {
		out.Items[i] = it.api
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateSummary generates a summary from the ticked items and saves it
// (§12.1). Unticked items never reach the model; client-facing summaries use
// decision records and Client-safe comments only (AC-TK-10).
func (s *Server) CreateSummary(w http.ResponseWriter, r *http.Request) {
	var in SummaryCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	scope := SummaryScope{Audience: in.Audience, ClientId: in.ClientId, From: in.From, To: in.To, IncludeCancelled: in.IncludeCancelled,
		Language: in.Language, NodeId: in.NodeId, ProjectKey: in.ProjectKey}
	pc, node, clientName, all, ok := s.summaryScope(w, r, scope)
	if !ok {
		return
	}
	var items []summaryItem
	for _, it := range all {
		if slices.Contains(in.Keys, it.api.Key) {
			items = append(items, it)
		}
	}
	if len(items) == 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields",
			FieldError{Field: "keys", Code: "min_items", Message: "Tick at least one item in scope"})
		return
	}
	ctx := r.Context()
	client := in.Audience == SummaryAudienceClient
	lang := string(in.Language)
	inputs, outItems, err := s.summaryInputs(ctx, items, client)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	res, err := draft.Summarize(ctx, s.ai, inputs, lang, client)
	if err != nil {
		s.draftFailed(w, r, err)
		return
	}
	title := draft.Title(node.Name, clientName, in.From.Time, in.To.Time, lang)
	md := draft.Markdown(title, res, inputs, lang, client)
	params, _ := json.Marshal(scope)
	itemsJSON, _ := json.Marshal(outItems)
	row, err := s.q.CreateSummary(ctx, db.CreateSummaryParams{
		ProjectID: pc.project.ID, CreatedBy: pc.user.ID, Title: title, Params: params, Items: itemsJSON, Markdown: md, Model: res.Model,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAPISummary(row, pc.project.Key, pc.user.Name))
}

// summaryInputs builds what the model reads for each item: the decision record,
// and for the team also the ticket's reason, description and latest five
// comments; a client-facing summary reads Client-safe comments only (AC-TK-10).
func (s *Server) summaryInputs(ctx context.Context, items []summaryItem, client bool) ([]draft.Item, []SummaryItem, error) {
	var ticketIDs []int64
	for _, it := range items {
		if it.ticketID != 0 {
			ticketIDs = append(ticketIDs, it.ticketID)
		}
	}
	comments, err := s.q.ListCommentsOfTickets(ctx, db.ListCommentsOfTicketsParams{TicketIds: ticketIDs, ClientSafe: client})
	if err != nil {
		return nil, nil, err
	}
	byTicket := map[int64][]string{}
	for _, c := range comments {
		byTicket[c.TicketID] = append(byTicket[c.TicketID], c.Body)
	}
	inputs := make([]draft.Item, len(items))
	outItems := make([]SummaryItem, len(items))
	for i, it := range items {
		outItems[i] = it.api
		var b strings.Builder
		if d := it.decision; d != nil {
			if d.State != nil {
				fmt.Fprintf(&b, "What changed: %s\nWhy: %s\n", deref(d.WhatChanged), deref(d.Why))
				if a := deref(d.Alternatives); a != "" {
					fmt.Fprintf(&b, "Alternatives rejected: %s\n", a)
				}
			}
			if !client {
				t, err := s.q.GetTicketByKey(ctx, d.Key)
				if err != nil {
					return nil, nil, err
				}
				if t.Ticket.Reason != "" {
					fmt.Fprintf(&b, "Reason: %s\n", t.Ticket.Reason)
				}
				if t.Ticket.Description != "" {
					fmt.Fprintf(&b, "Description: %s\n", t.Ticket.Description)
				}
			}
			cs := byTicket[it.ticketID]
			for _, c := range cs[max(0, len(cs)-5):] { // the latest five
				fmt.Fprintf(&b, "Comment: %s\n", c)
			}
		} else {
			b.WriteString(it.noteBody)
		}
		inputs[i] = draft.Item{Key: it.api.Key, Kind: string(it.api.Kind), Title: it.api.Title, Menu: it.api.Menu,
			Client: deref(it.api.Client), RequestedBy: deref(it.api.RequestedBy), Date: it.api.Date.Time,
			Cancelled: deref(it.api.Cancelled), Text: b.String(), AcceptedBy: deref(it.api.AcceptedBy)}
		if it.api.AcceptedOn != nil {
			inputs[i].AcceptedOn = it.api.AcceptedOn.Time
		}
		if d := it.decision; d != nil && d.State != nil {
			inputs[i].Change, inputs[i].Why, inputs[i].ReversedBy = deref(d.WhatChanged), deref(d.Why), deref(d.SupersededByKey)
		}
	}
	return inputs, outItems, nil
}

// summaryFor loads a summary for its creator, while still a member of its
// project, or a project admin; anyone else gets 404.
func (s *Server) summaryFor(w http.ResponseWriter, r *http.Request, id int64) (db.GetSummaryRow, bool) {
	u := s.requireUser(w, r)
	if u == nil {
		return db.GetSummaryRow{}, false
	}
	ctx := r.Context()
	row, err := s.q.GetSummary(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Summary not found")
		return db.GetSummaryRow{}, false
	}
	if err != nil {
		s.fail(w, r, err)
		return db.GetSummaryRow{}, false
	}
	if !u.IsAdmin {
		scope, member, err := access.ForProject(ctx, s.q, u, row.Summary.ProjectID)
		if err != nil {
			s.fail(w, r, err)
			return db.GetSummaryRow{}, false
		}
		need := access.Admin
		if row.Summary.CreatedBy == u.ID {
			need = access.Member
		}
		if !member || !scope.Allows(need) {
			writeProblem(w, http.StatusNotFound, "not_found", "Summary not found")
			return db.GetSummaryRow{}, false
		}
	}
	return row, true
}

// GetSummary reads one summary.
func (s *Server) GetSummary(w http.ResponseWriter, r *http.Request, id int64) {
	row, ok := s.summaryFor(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toAPISummary(row.Summary, row.ProjectKey, row.CreatorName))
}

// UpdateSummary saves an edited title and markdown; tickets never change.
func (s *Server) UpdateSummary(w http.ResponseWriter, r *http.Request, id int64) {
	row, ok := s.summaryFor(w, r, id)
	if !ok {
		return
	}
	var in SummaryUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	title := strings.TrimSpace(in.Title)
	if f := textRange("title", title, 1, 300, "Give the summary a title of up to 300 characters"); len(f) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", f...)
		return
	}
	saved, err := s.q.UpdateSummary(r.Context(), db.UpdateSummaryParams{ID: id, Title: title, Markdown: in.Markdown})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAPISummary(saved, row.ProjectKey, row.CreatorName))
}

// ListSummaries lists the caller's summaries and those of projects they administer.
func (s *Server) ListSummaries(w http.ResponseWriter, r *http.Request, params ListSummariesParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	var projectID *int64
	if params.Project != nil {
		p, err := s.q.GetProjectByKey(ctx, strings.ToUpper(*params.Project))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, SummaryList{Items: []SummaryListItem{}})
			return
		}
		if err != nil {
			s.fail(w, r, err)
			return
		}
		projectID = &p.ID
	}
	rows, err := s.q.ListSummaries(ctx, db.ListSummariesParams{ProjectID: projectID, IsAdmin: u.IsAdmin, UserID: u.ID})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := SummaryList{Items: make([]SummaryListItem, len(rows))}
	for i, x := range rows {
		out.Items[i] = SummaryListItem{Id: x.ID, Title: x.Title, ProjectKey: x.ProjectKey, Creator: x.CreatorName, CreatedAt: x.CreatedAt}
	}
	writeJSON(w, http.StatusOK, out)
}

func toAPISummary(x db.Summary, projectKey, creator string) Summary {
	out := Summary{Id: x.ID, ProjectKey: projectKey, Title: x.Title, Markdown: x.Markdown, Model: x.Model,
		CreatedBy: Ref{Id: x.CreatedBy, Name: creator}, CreatedAt: x.CreatedAt, UpdatedAt: x.UpdatedAt, Items: []SummaryItem{}}
	_ = json.Unmarshal(x.Params, &out.Scope)
	_ = json.Unmarshal(x.Items, &out.Items)
	return out
}
