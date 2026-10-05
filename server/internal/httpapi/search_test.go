package httpapi_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// FSD §6.1–6.2: search finds tickets by key, by words or by part of a title,
// and nodes by name, alias or code, only where the user may look (R-AC-7).
func TestSearchFindsTicketsAndNodesInScope(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ctx := context.Background()
	code := "HR.ATT.OT"
	if _, err := e.q.UpdateNode(ctx, db.UpdateNodeParams{ID: w.ot.ID, Code: &code, Aliases: []string{"Persetujuan Lembur"}}); err != nil {
		t.Fatal(err)
	}
	approval := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval for overtime", &w.a, w.ot)
	if _, err := e.d.Pool.Exec(ctx, "UPDATE tickets SET reason = 'HR approves lembur directly' WHERE id = $1", approval.ID); err != nil {
		t.Fatal(err)
	}
	e.seedTicket(w.p, w.pmUser, "Overtime report for Client B", &w.b, w.secret) // hidden from the PM, who sees Client A
	search := func(q string) (tickets, nodes []string) {
		t.Helper()
		var res httpapi.SearchResults
		if code := e.call(w.pm, http.MethodGet, "/search?q="+url.QueryEscape(q), nil, &res); code != http.StatusOK {
			t.Fatalf("search %q: %d", q, code)
		}
		for _, tk := range res.Tickets {
			tickets = append(tickets, tk.Key)
		}
		for _, n := range res.Nodes {
			nodes = append(nodes, n.Name)
		}
		return tickets, nodes
	}
	for _, c := range []struct {
		q              string
		tickets, nodes []string
	}{
		{"hris-1", []string{"HRIS-1"}, nil},                           // a key, in any case
		{"overt", []string{"HRIS-1"}, []string{"Overtime Approval"}},  // part of a title and of a name
		{"lembur", []string{"HRIS-1"}, []string{"Overtime Approval"}}, // a word in the reason, and an alias
		{"hr.att", nil, []string{"Overtime Approval"}},                // a code
		{"report", nil, nil},                                          // Client B's ticket and menu stay hidden
		{"o", nil, nil},                                               // too short to search
	} {
		tickets, nodes := search(c.q)
		if !slices.Equal(tickets, c.tickets) || !slices.Equal(nodes, c.nodes) {
			t.Errorf("search %q: %v %v, want %v %v", c.q, tickets, nodes, c.tickets, c.nodes)
		}
	}
	var res httpapi.SearchResults
	e.call(w.pm, http.MethodGet, "/search?q=overtime", nil, &res)
	if len(res.Nodes) != 1 || !slices.Equal(res.Nodes[0].Path, []string{"HR", "Overtime Approval"}) || res.Nodes[0].ProjectKey != "HRIS" ||
		res.Nodes[0].Code == nil || *res.Nodes[0].Code != code || !slices.Equal(res.Nodes[0].Aliases, []string{"Persetujuan Lembur"}) {
		t.Fatalf("the node result: %+v", res.Nodes)
	}
}
