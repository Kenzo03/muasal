package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/httpapi"
)

// MSL-66: a member records that Budi of Client A accepted the ticket's work;
// the ticket shows it, the list filters on it, and a summary's appendix names
// it. The contact must be the ticket's client's, the day today or earlier.
func TestClientAcceptance(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Skip supervisor approval", &w.a, w.ot)
	e.seedTicket(w.p, w.pmUser, "Still waiting", &w.a, w.ot)
	other, err := e.q.CreateContact(context.Background(), db.CreateContactParams{ClientID: &w.b.ID, Name: "Sari"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/tickets/" + tk.Key + "/acceptance"
	tomorrow := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPut, path, map[string]any{"contact_id": other, "accepted_on": tomorrow}, &p); code != http.StatusUnprocessableEntity ||
		len(*p.Errors) != 2 || firstError(p).Field != "contact_id" {
		t.Fatalf("bad acceptance: %d %+v", code, p)
	}
	var got httpapi.Ticket
	if code := e.call(w.pm, http.MethodPut, path, map[string]any{"contact_id": w.budi, "accepted_on": "2025-12-12", "note": "UAT signed in the steering meeting"}, &got); code != http.StatusOK ||
		got.Acceptance == nil || got.Acceptance.Contact.Name != "Budi" || got.Acceptance.AcceptedOn.String() != "2025-12-12" || got.Acceptance.Note != "UAT signed in the steering meeting" {
		t.Fatalf("accept: %d %+v", code, got.Acceptance)
	}

	var page httpapi.TicketPage
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?accepted=true", nil, &page); len(page.Items) != 1 || page.Items[0].Key != tk.Key || page.Items[0].AcceptedOn == nil {
		t.Fatalf("accepted: %+v", page.Items)
	}
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/tickets?accepted=false", nil, &page); len(page.Items) != 1 || page.Items[0].Key == tk.Key {
		t.Fatalf("not accepted: %+v", page.Items)
	}

	e.seedClose(tk, statusID(e, w.pm, "Done"), w.pmUser, "HR approves overtime directly.")
	scope := map[string]any{"project_key": "HRIS", "node_id": 0, "from": "2020-01-01", "to": time.Now().AddDate(0, 0, 1).Format("2006-01-02"), "language": "en", "audience": "internal"}
	var prev httpapi.SummaryPreview
	if e.call(w.pm, http.MethodPost, "/summaries/preview", scope, &prev); len(prev.Items) != 1 || prev.Items[0].AcceptedBy == nil || *prev.Items[0].AcceptedBy != "Budi" || prev.Items[0].AcceptedOn == nil || prev.Items[0].AcceptedOn.String() != "2025-12-12" {
		t.Fatalf("summary item: %+v", prev.Items)
	}

	if code := e.call(w.pm, http.MethodDelete, path, nil, &got); code != http.StatusOK {
		t.Fatalf("unaccept: %d", code)
	}
	var fresh httpapi.Ticket
	if e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &fresh); fresh.Acceptance != nil {
		t.Fatalf("still accepted: %+v", fresh.Acceptance)
	}
	events, err := e.q.ListAuditEvents(t.Context(), db.ListAuditEventsParams{Entity: "ticket", EntityID: tk.ID})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, ev := range events {
		actions = append(actions, ev.Action)
	}
	if s := strings.Join(actions, ","); !strings.Contains(s, "accept,") || !strings.HasSuffix(s, "unaccept") {
		t.Fatalf("history: %s", s)
	}
}
