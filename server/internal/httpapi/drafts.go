package httpapi

import (
	"errors"
	"net/http"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/draft"
)

// DraftDecision drafts a ticket's decision record with the chat model from
// its thread (FSD §9.3). Nothing is saved; the close dialog fills only empty
// fields, and the record is saved by "Close ticket".
func (s *Server) DraftDecision(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in DraftDecisionJSONBody
	if r.ContentLength != 0 && !decodeJSON(w, r, &in) {
		return
	}
	lang := "id"
	if in.Language != nil {
		lang = string(*in.Language)
	} else if pc.user.Locale == "en" {
		lang = "en"
	}
	ctx := r.Context()
	t := row.Ticket
	th := draft.Thread{Key: t.Key, Title: t.Title, Type: t.Type, Client: deref(row.ClientName), Reason: t.Reason, Description: t.Description}
	menus, err := s.q.ListTicketNodes(ctx, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, m := range menus {
		th.Menus = append(th.Menus, m.Name)
	}
	comments, err := s.q.ListDraftComments(ctx, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, c := range comments {
		th.Comments = append(th.Comments, draft.Comment{Author: c.AuthorName, At: c.CreatedAt, Body: c.Body})
	}
	commits, err := s.q.ListTicketCommits(ctx, t.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, c := range commits {
		th.Commits = append(th.Commits, c.Message)
	}
	d, model, err := draft.DraftDecision(ctx, s.ai, th, lang)
	if err != nil {
		s.draftFailed(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, DecisionDraft{WhatChanged: d.WhatChanged, Why: d.Why, Alternatives: d.Alternatives, Model: model})
}

// draftFailed answers a model failure: AI off is 409, the rest 503.
func (s *Server) draftFailed(w http.ResponseWriter, r *http.Request, err error) {
	var de *draft.Error
	switch {
	case errors.As(err, &de) && de.Code == "ai_off":
		writeProblem(w, http.StatusConflict, "ai_off", "AI is turned off")
	case errors.As(err, &de):
		writeProblem(w, http.StatusServiceUnavailable, de.Code, askErrors[de.Code])
	default:
		s.fail(w, r, err)
	}
}
