package httpapi

import (
	openapi_types "github.com/oapi-codegen/runtime/types"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kenzo03/zettra/server/internal/db"
)

// Search finds the tickets, nodes and decision notes the user may open, across their projects
// (FSD §6.1, §6.2). The membership check runs in SQL, so a hidden row never
// leaves the database (R-AC-7); the web jumps straight to a ticket whose key
// matches.
func (s *Server) Search(w http.ResponseWriter, r *http.Request, params SearchParams) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	q := strings.TrimSpace(params.Q)
	if utf8.RuneCountInString(q) > 200 {
		writeProblem(w, http.StatusBadRequest, "invalid_parameter", "Search for at most 200 characters")
		return
	}
	out := SearchResults{Tickets: []SearchTicket{}, Nodes: []SearchNode{}, Notes: []SearchNote{}, Sections: []SearchSection{}}
	if utf8.RuneCountInString(q) < 2 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := r.Context()
	tickets, err := s.q.SearchTickets(ctx, db.SearchTicketsParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	nodes, err := s.q.SearchNodes(ctx, db.SearchNodesParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	notes, err := s.q.SearchNotes(ctx, db.SearchNotesParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sections, err := s.q.SearchSections(ctx, db.SearchSectionsParams{IsAdmin: u.IsAdmin, UserID: u.ID, Q: q})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, sec := range sections {
		out.Sections = append(out.Sections, SearchSection{DocumentKey: sec.DocumentKey, DocumentTitle: sec.DocumentTitle, ProjectKey: sec.ProjectKey,
			Number: sec.Number, Title: sec.Title, Excerpt: excerpt(sec.Body, q), Superseded: sec.Superseded})
	}
	for _, n := range notes {
		out.Notes = append(out.Notes, SearchNote{Key: n.Key, Title: n.Title, ProjectKey: n.ProjectKey, DecidedOn: openapi_types.Date{Time: n.DecidedOn}})
	}
	for _, t := range tickets {
		it := SearchTicket{Key: t.Key, Title: t.Title, ProjectKey: t.ProjectKey, Status: toAPIStatus(t.Status)}
		if t.ClientID != nil {
			it.Client = &Ref{Id: *t.ClientID, Name: deref(t.ClientName)}
		}
		out.Tickets = append(out.Tickets, it)
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, SearchNode{
			Id: n.ID, Name: n.Name, Type: NodeType(n.Type), Code: n.Code, Aliases: orEmpty(n.Aliases),
			ProjectKey: n.ProjectKey, Path: orEmpty(n.Path),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// excerpt is plain text around the first word of q found in body, so a result
// shows why it matched (MSL-15); the start of the body when none is.
func excerpt(body, q string) string {
	text := []rune(strings.Join(strings.Fields(body), " "))
	lower := []rune(strings.ToLower(string(text)))
	at := 0
	for _, word := range strings.Fields(strings.ToLower(q)) {
		if i := strings.Index(string(lower), word); utf8.RuneCountInString(word) > 1 && i >= 0 {
			at = utf8.RuneCountInString(string(lower)[:i])
			break
		}
	}
	from, to := max(0, at-60), min(len(text), at+140)
	out := string(text[from:to])
	if from > 0 {
		out = "…" + out
	}
	if to < len(text) {
		out += "…"
	}
	return out
}
