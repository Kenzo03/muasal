package httpapi

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

// GetHandover gathers the handover pack (MSL-68): every live module and menu
// in tree order, each with its own spec sections in force, behaviours in
// force and open tickets, as far as the caller sees them; with a client, that
// client's items and the core work.
// ponytail: three queries per node; one query per kind if trees grow past a few hundred nodes.
func (s *Server) GetHandover(w http.ResponseWriter, r *http.Request, key string, params GetHandoverParams) {
	pc, ok := s.projectFor(w, r, key, access.Viewer)
	if !ok {
		return
	}
	ctx := r.Context()
	nodes, err := s.q.ListNodes(ctx, db.ListNodesParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	statuses, err := s.q.ListStatuses(ctx, pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	statusName := map[int64]string{}
	for _, st := range statuses {
		statusName[st.ID] = st.Name
	}
	forClient := func(id *int64) bool { return params.ClientId == nil || id == nil || *id == *params.ClientId }
	children := map[int64][]db.ListNodesRow{} // by parent; 0 holds the top modules
	for _, n := range nodes {
		p := int64(0)
		if n.ParentID != nil {
			p = *n.ParentID
		}
		children[p] = append(children[p], n)
	}
	out := Handover{Nodes: []HandoverNode{}}
	var walk func(parent int64, depth int) error
	walk = func(parent int64, depth int) error {
		for _, n := range children[parent] {
			one := []int64{n.ID}
			hn := HandoverNode{Id: n.ID, Name: n.Name, Code: n.Code, Type: HandoverNodeType(n.Type), Depth: depth,
				Sections: []TimelineSection{}, Behaviors: []Behavior{}, Open: []HandoverTicket{}}
			sections, err := s.q.ListNodeSections(ctx, db.ListNodeSectionsParams{ProjectID: pc.project.ID, NodeIds: one, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
			if err != nil {
				return err
			}
			for _, sec := range sections {
				if sec.SupersededByKey == nil && forClient(sec.ClientID) {
					hn.Sections = append(hn.Sections, toTimelineSection(sec))
				}
			}
			behaviors, err := s.q.ListNodeBehaviors(ctx, db.ListNodeBehaviorsParams{ProjectID: pc.project.ID, NodeIds: one, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs)})
			if err != nil {
				return err
			}
			for _, b := range behaviors {
				if forClient(b.ClientID) {
					hn.Behaviors = append(hn.Behaviors, toAPIBehavior(b))
				}
			}
			open, err := s.q.ListTickets(ctx, db.ListTicketsParams{ProjectID: pc.project.ID, AllClients: pc.scope.AllClients, ClientIds: orEmpty(pc.scope.ClientIDs),
				OpenOnly: true, NodeIds: one, Sort: "priority", Today: s.today(pc.user), Lim: 500})
			if err != nil {
				return err
			}
			for _, t := range open {
				if !forClient(t.ClientID) {
					continue
				}
				ht := HandoverTicket{Key: t.Key, Title: t.Title, Status: statusName[t.StatusID], Assignee: t.AssigneeName}
				if t.DueDate != nil {
					ht.DueDate = &openapi_types.Date{Time: *t.DueDate}
				}
				hn.Open = append(hn.Open, ht)
			}
			out.Nodes = append(out.Nodes, hn)
			if err := walk(n.ID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(0, 0); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
