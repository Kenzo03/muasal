package httpapi_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/zettra/server/internal/httpapi"
)

func notes(e *env, c *http.Client) httpapi.NotificationList {
	e.t.Helper()
	var out httpapi.NotificationList
	e.call(c, http.MethodGet, "/notifications", nil, &out)
	return out
}

func types(l httpapi.NotificationList) []string {
	out := []string{}
	for _, n := range l.Items {
		out = append(out, string(n.Type))
	}
	return out
}

// §8.10: assignment, comments, @mentions and status changes notify the
// right people once, never the actor, and only those who can see the ticket.
func TestTicketEventsNotify(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	rina, rinaUser := e.signedIn("rina@example.com", false)
	e.seedMember(rinaUser, w.p, "member", w.a)
	outsider, outsiderUser := e.signedIn("bayu@example.com", false)
	e.seedMember(outsiderUser, w.p, "member", w.b)

	var tk httpapi.Ticket
	e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "bug", "title": "Overtime approval stuck", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "assignee_id": rinaUser.ID,
	}, &tk)
	if got := notes(e, rina); got.Unread != 1 || fmt.Sprint(types(got)) != "[assigned]" || got.Items[0].Actor.Id != w.pmUser.ID ||
		*got.Items[0].TicketKey != tk.Key {
		t.Fatalf("assignee: %+v", got)
	}
	if got := notes(e, w.pm); len(got.Items) != 0 {
		t.Fatalf("the actor was notified: %+v", got)
	}

	// Rina mentions the PM and the outsider; the PM hears a mention (not also a comment); the outsider cannot see the ticket.
	e.call(rina, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "@pm can you check? cc @bayu"}, nil)
	if got := notes(e, w.pm); fmt.Sprint(types(got)) != "[mention]" || !strings.Contains(fmt.Sprint(got.Items[0].Payload["excerpt"]), "can you check") {
		t.Fatalf("mentioned reporter: %+v", got)
	}
	if got := notes(e, outsider); len(got.Items) != 0 {
		t.Fatalf("a member who cannot see the ticket: %+v", got)
	}
	// The PM replies: Rina, an earlier commenter and the assignee, hears a comment.
	e.call(w.pm, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Looking now."}, nil)
	if got := notes(e, rina); fmt.Sprint(types(got)) != "[comment assigned]" {
		t.Fatalf("assignee after a comment: %+v", types(got))
	}

	// A status change tells the reporter and the assignee, except whoever changed it.
	e.call(rina, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]any{"status_id": statusID(e, w.pm, "In progress")}, nil)
	got := notes(e, w.pm)
	if got.Items[0].Type != httpapi.NotificationTypeStatus || got.Items[0].Payload["status"] != "In progress" {
		t.Fatalf("status: %+v", got.Items[0])
	}

	// One read, then all.
	e.call(w.pm, http.MethodPost, "/notifications/read", map[string]any{"id": got.Items[0].Id}, nil)
	if n := notes(e, w.pm).Unread; n != 1 {
		t.Fatalf("after reading one: %d unread", n)
	}
	e.call(w.pm, http.MethodPost, "/notifications/read", map[string]any{}, nil)
	if n := notes(e, w.pm).Unread; n != 0 {
		t.Fatalf("after reading all: %d unread", n)
	}
}

// §8.10: each event can be turned off in the profile.
func TestNotificationPreferences(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	rina, rinaUser := e.signedIn("rina@example.com", false)
	e.seedMember(rinaUser, w.p, "member", w.a)
	var me httpapi.User
	if code := e.call(rina, http.MethodPatch, "/me", map[string]any{"notify_prefs": map[string]bool{"assigned": false, "browser": true}}, &me); code != http.StatusOK ||
		me.NotifyPrefs == nil || me.NotifyPrefs.Assigned == nil || *me.NotifyPrefs.Assigned || !*me.NotifyPrefs.Browser {
		t.Fatalf("prefs: %d %+v", code, me.NotifyPrefs)
	}
	e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "bug", "title": "Assigned quietly", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "assignee_id": rinaUser.ID,
	}, nil)
	if got := notes(e, rina); len(got.Items) != 0 {
		t.Fatalf("an event turned off: %+v", got)
	}
}

// AC-TK-9 (server side): a notification reaches an open stream within 5 seconds.
func TestNotificationsStreamToOpenTabs(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	rina, rinaUser := e.signedIn("rina@example.com", false)
	e.seedMember(rinaUser, w.p, "member", w.a)
	req, _ := http.NewRequest(http.MethodGet, e.url+"/api/v1/notifications/stream", nil)
	res, err := rina.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	events := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(res.Body)
		var event string
		for sc.Scan() {
			line := sc.Text()
			if v, ok := strings.CutPrefix(line, "event: "); ok {
				event = v
			} else if v, ok := strings.CutPrefix(line, "data: "); ok && event == "notification" {
				events <- v
			} else if v, ok := strings.CutPrefix(line, "data: "); ok && event == "ready" {
				events <- "ready" + v
			}
		}
	}()
	select {
	case <-events: // ready: subscribed
	case <-time.After(5 * time.Second):
		t.Fatal("the stream never opened")
	}
	time.Sleep(200 * time.Millisecond) // the listener's LISTEN is in place
	e.call(w.pm, http.MethodPost, "/projects/HRIS/tickets", map[string]any{
		"type": "bug", "title": "Assigned live", "node_ids": []int64{w.ot.ID}, "client_id": w.a.ID, "assignee_id": rinaUser.ID,
	}, nil)
	select {
	case data := <-events:
		var n httpapi.Notification
		if err := json.Unmarshal([]byte(data), &n); err != nil || n.Type != httpapi.NotificationTypeAssigned || *n.TicketTitle != "Assigned live" {
			t.Fatalf("pushed: %v %s", err, data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification within 5 seconds")
	}
}

// MSL-57: someone not on a ticket follows it, hears its comments and status
// changes, and stops hearing them after unfollowing.
func TestFollowersHearATicket(t *testing.T) {
	e := newEnv(t)
	w := newHRIS(e)
	qa, qu := e.signedIn("qa@example.com", false)
	e.seedMember(qu, w.p, "member")
	tk := e.seedTicket(w.p, w.pmUser, "Cut-off payroll uses the 30th", &w.a, w.ot)
	if code := e.call(qa, http.MethodPost, "/tickets/"+tk.Key+"/follow", nil, nil); code != http.StatusNoContent {
		t.Fatalf("follow: %d", code)
	}
	var got httpapi.Ticket
	if e.call(qa, http.MethodGet, "/tickets/"+tk.Key, nil, &got); got.Following == nil || !*got.Following {
		t.Fatalf("following: %+v", got.Following)
	}
	admin, _ := e.signedIn("admin@example.com", true)
	e.call(admin, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Fixed on staging."}, nil)
	var statuses httpapi.StatusList
	e.call(admin, http.MethodGet, "/projects/HRIS/statuses", nil, &statuses)
	var inProgress int64
	for _, s := range statuses.Items {
		if s.Category == httpapi.StatusCategoryInProgress {
			inProgress = s.Id
			break
		}
	}
	e.call(admin, http.MethodPost, "/tickets/"+tk.Key+"/transition", map[string]any{"status_id": inProgress}, nil)
	if got := types(notes(e, qa)); !slices.Contains(got, "comment") || !slices.Contains(got, "status") {
		t.Fatalf("a follower's notifications: %v", got)
	}
	if code := e.call(qa, http.MethodDelete, "/tickets/"+tk.Key+"/follow", nil, nil); code != http.StatusNoContent {
		t.Fatalf("unfollow: %d", code)
	}
	before := len(notes(e, qa).Items)
	e.call(admin, http.MethodPost, "/tickets/"+tk.Key+"/comments", map[string]any{"body": "Released."}, nil)
	if after := len(notes(e, qa).Items); after != before {
		t.Fatalf("an unfollower still hears: %d → %d", before, after)
	}
}
