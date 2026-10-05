package httpapi_test

import (
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

func activity(e *env, c *http.Client, key string) []httpapi.ActivityItem {
	e.t.Helper()
	var list httpapi.ActivityList
	if code := e.call(c, http.MethodGet, "/tickets/"+key+"/activity", nil, &list); code != http.StatusOK {
		e.t.Fatalf("activity: %d", code)
	}
	return list.Items
}

func TestCommentsDefaultToInternalAndEditsKeepTheirHistory(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var c httpapi.ActivityItem
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Budi confirmed by phone."}, &c); code != http.StatusCreated ||
		c.Internal == nil || !*c.Internal {
		t.Fatalf("a comment is Internal by default (AC-TK-10): %d %+v", code, c)
	}
	path := fmt.Sprintf("/comments/%d", *c.CommentId)
	for _, body := range []string{"Budi confirmed by phone on Monday.", "Budi confirmed by phone on Monday, 9 am."} {
		if code := e.call(w.pm, http.MethodPatch, path, map[string]any{"body": body}, nil); code != http.StatusOK {
			t.Fatalf("edit: %d", code)
		}
	}
	// AC-TK-6: both earlier versions appear, with actor and time.
	var olds []string
	for _, it := range activity(e, w.pm, tk.Key) {
		if it.Action != nil && *it.Action == "comment_edit" {
			olds = append(olds, (*it.Changes)["body"].(map[string]any)["old"].(string))
			if it.Actor == nil || it.Actor.Name != "pm@example.com" {
				t.Fatalf("edit without its actor: %+v", it)
			}
		}
	}
	if !slices.Equal(olds, []string{"Budi confirmed by phone.", "Budi confirmed by phone on Monday."}) {
		t.Fatalf("earlier versions: %v", olds)
	}
	if code := e.call(w.pm, http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	for _, reader := range []struct {
		c       *http.Client
		readsIt bool
	}{{w.pm, false}, {admin, true}} {
		for _, it := range activity(e, reader.c, tk.Key) {
			if it.Kind == httpapi.ActivityItemKindComment && (!*it.Deleted || (it.Body != nil) != reader.readsIt) {
				t.Errorf("deleted comment: %+v", it)
			}
		}
	}
}

func TestOnlyAuthorsChangeTheirComments(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	ani, au := e.signedIn("ani@example.com", false)
	e.seedMember(au, w.p, "member")
	vera, vu := e.signedIn("vera@example.com", false)
	e.seedMember(vu, w.p, "viewer")
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var c httpapi.ActivityItem
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Client-safe note", "internal": false}, &c)
	if c.Internal == nil || *c.Internal {
		t.Fatalf("client-safe comment: %+v", c)
	}
	path := fmt.Sprintf("/comments/%d", *c.CommentId)
	if code := e.call(ani, http.MethodPatch, path, map[string]any{"body": "Not mine"}, nil); code != http.StatusForbidden {
		t.Errorf("another member edits: %d", code)
	}
	if code := e.call(ani, http.MethodDelete, path, nil, nil); code != http.StatusForbidden {
		t.Errorf("another member deletes: %d", code)
	}
	if code := e.call(vera, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Hello"}, nil); code != http.StatusForbidden {
		t.Errorf("a viewer comments: %d", code)
	}
	if code := e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "   "}, nil); code != http.StatusUnprocessableEntity {
		t.Errorf("an empty comment: %d", code)
	}
	hidden := e.seedTicket(w.p, au, "Client B payroll change", &w.b)
	if code := e.call(w.pm, http.MethodGet, "/tickets/"+hidden.Key+"/activity", nil, nil); code != http.StatusNotFound {
		t.Errorf("a hidden ticket's activity: %d", code)
	}
}

func TestActivityInterleavesCommentsAndHistory(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var st httpapi.StatusList
	e.call(w.pm, http.MethodGet, "/projects/HRIS/statuses", nil, &st)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Starting now"}, nil)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]any{"status_id": st.Items[1].Id}, nil)
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Halfway"}, nil)
	var kinds []string
	for _, it := range activity(e, w.pm, tk.Key) {
		kind := string(it.Kind)
		if it.Action != nil {
			kind = *it.Action
		}
		kinds = append(kinds, kind)
	}
	if !slices.Equal(kinds, []string{"comment", "transition", "comment"}) {
		t.Fatalf("activity: %v", kinds)
	}
}

// FSD §8.7: once a comment is deleted, its edits no longer carry its text for
// readers other than system admins, nor does an edit of a comment not on the
// ticket; Home's latest change never carries it.
func TestDeletedCommentsLeaveNoTextInTheirEdits(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Overtime export", &w.a)
	var c httpapi.ActivityItem
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Salary of Budi is 9m"}, &c)
	path := fmt.Sprintf("/comments/%d", *c.CommentId)
	if code := e.call(w.pm, http.MethodPatch, path, map[string]any{"body": "Budi confirmed"}, nil); code != http.StatusOK {
		t.Fatalf("edit: %d", code)
	}
	for _, reader := range []*http.Client{w.pm, admin} {
		var home httpapi.RecentTicketList
		e.call(reader, http.MethodGet, "/me/updates", nil, &home)
		if len(home.Items) != 1 || home.Items[0].Change == nil || home.Items[0].Change.Changes == nil {
			t.Fatalf("home: %+v", home.Items)
		} else if _, ok := (*home.Items[0].Change.Changes)["body"]; ok {
			t.Fatalf("home carries the text: %+v", *home.Items[0].Change.Changes)
		}
	}
	e.exec(`INSERT INTO audit_events (actor_id, via, entity, entity_id, project_id, action, changes)
		VALUES ($1, 'web', 'ticket', $2, $3, 'comment_edit', '{"comment_id": 999999, "body": {"old": "x", "new": "y"}}')`, w.pmUser.ID, tk.ID, w.p.ID)
	if code := e.call(w.pm, http.MethodDelete, path, nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	for _, reader := range []struct {
		c       *http.Client
		readsIt bool
	}{{w.pm, false}, {admin, true}} {
		edits := 0
		for _, it := range activity(e, reader.c, tk.Key) {
			if it.Action != nil && *it.Action == "comment_edit" {
				edits++
				if _, ok := (*it.Changes)["body"]; ok != reader.readsIt {
					t.Errorf("edit of a deleted comment (admin %v): %+v", reader.readsIt, *it.Changes)
				}
			}
		}
		if edits != 2 {
			t.Errorf("edits: %d", edits)
		}
	}
}
