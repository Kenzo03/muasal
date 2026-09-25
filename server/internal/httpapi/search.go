package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Search finds the tickets and nodes the user may open, across their projects
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
	out := SearchResults{Tickets: []SearchTicket{}, Nodes: []SearchNode{}}
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
