package httpapi_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// MSL-67: a PM adds v1.0 and v1.1, files tickets into them, finds v1.0's
// tickets in the list, and a v1.0 summary lists v1.0's tickets only.
func TestReleases(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	var v10, v11 httpapi.Release
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/releases", map[string]any{"name": " v1.0 "}, &v10); code != http.StatusCreated || v10.Name != "v1.0" || v10.ReleasedOn != nil {
		t.Fatalf("create: %d %+v", code, v10)
	}
	var p httpapi.Problem
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/releases", map[string]any{"name": "V1.0"}, &p); code != http.StatusUnprocessableEntity || firstError(p).Code != "taken" {
		t.Fatalf("same name: %d %+v", code, p)
	}
	e.call(w.pm, http.MethodPost, "/projects/HRIS/releases", map[string]any{"name": "v1.1"}, &v11)
	if code := e.call(w.pm, http.MethodPatch, fmt.Sprintf("/releases/%d", v10.Id), map[string]any{"name": "v1.0", "released_on": "2026-03-01"}, &v10); code != http.StatusOK || v10.ReleasedOn == nil || v10.ReleasedOn.String() != "2026-03-01" {
		t.Fatalf("ship: %d %+v", code, v10)
	}
	var list httpapi.ReleaseList
	if e.call(w.pm, http.MethodGet, "/projects/HRIS/releases", nil, &list); len(list.Items) != 2 || list.Items[0].Name != "v1.1" {
		t.Fatalf("list: %+v", list.Items)
	}

	file := func(title string, release int64) httpapi.Ticket {
		var tk httpapi.Ticket
		body := map[string]any{"client_id": w.a.ID, "title": title, "node_ids": []int64{w.ot.ID}, "type": "change_request", "reason": "Asked by HR", "release_id": release}
		if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &tk); code != http.StatusCreated || tk.Release == nil || tk.Release.Id != release {
			t.Fatalf("file %q: %d %+v", title, code, tk.Release)
		}
		return tk
	}
	a, b := file("Overtime approval by HR", v10.Id), file("Overtime export", v11.Id)
	var page httpapi.TicketPage
	if e.call(w.pm, http.MethodGet, fmt.Sprintf("/projects/HRIS/tickets?release_id=%d", v10.Id), nil, &page); len(page.Items) != 1 || page.Items[0].Key != a.Key || page.Items[0].Release == nil || *page.Items[0].Release != "v1.0" {
		t.Fatalf("filter: %+v", page.Items)
	}

	done := statusID(e, w.pm, "Done")
	for _, k := range []string{a.Key, b.Key} {
		row, err := e.q.GetTicketByKey(t.Context(), k)
		if err != nil {
			t.Fatal(err)
		}
		e.seedClose(row.Ticket, done, w.pmUser, "Shipped.")
	}
	scope := map[string]any{"project_key": "HRIS", "node_id": 0, "release_id": v10.Id, "from": "2020-01-01", "to": time.Now().AddDate(0, 0, 1).Format("2006-01-02"), "language": "en", "audience": "internal"}
	var prev httpapi.SummaryPreview
	if code := e.call(w.pm, http.MethodPost, "/summaries/preview", scope, &prev); code != http.StatusOK || len(prev.Items) != 1 || prev.Items[0].Key != a.Key {
		t.Fatalf("v1.0 summary: %d %+v", code, prev.Items)
	}

	other := e.seedProject("DMS")
	e.seedMember(w.pmUser, other, "member")
	var foreign httpapi.Release
	e.call(w.pm, http.MethodPost, "/projects/DMS/releases", map[string]any{"name": "v2"}, &foreign)
	body := map[string]any{"client_id": w.a.ID, "title": "Wrong release", "node_ids": []int64{w.ot.ID}, "type": "bug", "release_id": foreign.Id}
	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", body, &p); code != http.StatusUnprocessableEntity || firstError(p).Field != "release_id" {
		t.Fatalf("another project's release: %d %+v", code, p)
	}
}
