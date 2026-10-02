package httpapi_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/config"
	"github.com/kenzo03/muasal/server/internal/gitlink"
	"github.com/kenzo03/muasal/server/internal/httpapi"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

func (e *env) deliver(repoID int64, headers map[string]string, body []byte) int {
	e.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/webhooks/git/%d", e.url, repoID), bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

// processAll runs the delivery jobs the way `app serve` would.
func (e *env) processAll() {
	e.t.Helper()
	rows, err := e.d.Pool.Query(context.Background(), "SELECT id FROM webhook_deliveries ORDER BY id")
	if err != nil {
		e.t.Fatal(err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		e.t.Fatal(err)
	}
	for _, id := range ids {
		if err := gitlink.Process(context.Background(), e.d.Pool, id, func(context.Context, pgx.Tx, []int64) error { return nil }); err != nil {
			e.t.Fatal(err)
		}
	}
}

func sign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// §14.1, AC-IN-1 and AC-IN-2: a GitHub push naming HRIS-1 shows on the ticket
// and in its chunks; a bad signature is 401 and audited; a re-delivery
// changes nothing.
func TestGitHubPushLinksCommits(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{7}, 32) })
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Supervisor skip", &w.a, w.ot)

	if code := e.call(w.pm, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "github", "name": "hris-app", "web_url": "https://github.com/acme/hris-app"}, nil); code != http.StatusForbidden {
		t.Fatalf("a member adds a repo: %d", code)
	}
	var repo httpapi.Repo
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "github", "name": "hris-app", "web_url": "https://github.com/acme/hris-app"}, &repo); code != http.StatusCreated ||
		repo.Secret == nil || !strings.HasSuffix(repo.WebhookUrl, fmt.Sprintf("/webhooks/git/%d", repo.Id)) {
		t.Fatalf("create: %d %+v", code, repo)
	}
	var list httpapi.RepoList
	if e.call(admin, http.MethodGet, "/projects/HRIS/repos", nil, &list); len(list.Items) != 1 || list.Items[0].Secret != nil {
		t.Fatalf("the secret shows again: %+v", list)
	}

	push := []byte(`{"ref":"refs/heads/main","commits":[{"id":"abc1234def","message":"HRIS-1 fix supervisor skip","timestamp":"2026-09-20T10:00:00Z","url":"https://github.com/acme/hris-app/commit/abc1234def","author":{"name":"PM","email":"pm@example.com"}},{"id":"fff0000","message":"chore: bump deps","author":{"name":"Bot","email":"bot@example.com"}}]}`)
	if code := e.deliver(repo.Id, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign("wrong", push)}, push); code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", code)
	}
	var rejected int
	e.d.Pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'webhook_rejected'").Scan(&rejected)
	if rejected != 1 {
		t.Fatalf("rejections audited: %d", rejected)
	}
	if code := e.deliver(999999, map[string]string{"X-GitHub-Event": "push"}, push); code != http.StatusUnauthorized {
		t.Fatalf("unknown repo: %d", code)
	}
	if code := e.deliver(repo.Id, map[string]string{"X-GitHub-Event": "push"}, bytes.Repeat([]byte("x"), 5<<20+1)); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large: %d", code)
	}
	good := map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(*repo.Secret, push)}
	for range 2 { // the second delivery is a re-delivery
		if code := e.deliver(repo.Id, good, push); code != http.StatusAccepted {
			t.Fatalf("delivery: %d", code)
		}
		e.processAll()
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Code == nil || len(got.Code.Commits) != 1 || got.Code.Commits[0].Message != "HRIS-1 fix supervisor skip" || got.Code.Commits[0].Repo != "hris-app" {
		t.Fatalf("code: %+v", got.Code)
	}
	// Ask can use the message: it is in the ticket's chunks, and the author is found by the Person filter.
	if err := indexer.New(e.d.Pool, e.api.AI()).Rebuild(t.Context(), tk.ID); err != nil {
		t.Fatal(err)
	}
	var content string
	var users []int64
	if err := e.d.Pool.QueryRow(t.Context(), "SELECT content, user_ids FROM chunks WHERE ticket_id = $1 AND source_type = 'code'", tk.ID).Scan(&content, &users); err != nil ||
		!strings.Contains(content, "Commit hris-app abc1234 by PM on 2026-09-20: HRIS-1 fix supervisor skip") {
		t.Fatalf("code chunk: %v %q", err, content)
	}
}

// MSL-29: "Fixes HRIS-1" in a push moves the open ticket to In review, once,
// and its history says a webhook did it; it never closes the ticket.
func TestFixesInACommitMovesTheTicketToReview(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{7}, 32) })
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Supervisor skip", &w.a, w.ot)
	var repo httpapi.Repo
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "github", "name": "hris-app", "web_url": "https://github.com/acme/hris-app"}, &repo); code != http.StatusCreated {
		t.Fatalf("repo: %d", code)
	}
	push := []byte(`{"ref":"refs/heads/main","commits":[{"id":"abc1234def","message":"Fixes HRIS-1: skip the supervisor","timestamp":"2026-09-20T10:00:00Z","author":{"name":"PM","email":"pm@example.com"}}]}`)
	for range 2 { // the second delivery is a re-delivery
		if code := e.deliver(repo.Id, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(*repo.Secret, push)}, push); code != http.StatusAccepted {
			t.Fatalf("delivery: %d", code)
		}
		e.processAll()
	}
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Status.Name != "In review" || got.ClosedAt != nil {
		t.Fatalf("status: %+v closed %v", got.Status, got.ClosedAt)
	}
	var n int
	if err := e.d.Pool.QueryRow(t.Context(), `SELECT count(*) FROM audit_events
		WHERE entity = 'ticket' AND entity_id = $1 AND action = 'transition' AND via = 'webhook' AND changes->'status'->>'new' = 'In review'`, tk.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("history: %v %d transitions", err, n)
	}
}

// §14.1: GitLab authenticates with X-Gitlab-Token; a merge request links by
// its title or branch and keeps its state.
func TestGitLabMergeRequestLinks(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{7}, 32) })
	w := newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	tk := e.seedTicket(w.p, w.pmUser, "Supervisor skip", &w.a, w.ot)
	var repo httpapi.Repo
	e.call(admin, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "gitlab", "name": "hris-api", "web_url": "https://gitlab.example.com/acme/hris-api"}, &repo)
	mr := []byte(`{"object_attributes":{"iid":12,"title":"Skip supervisor for Client A","description":"Closes HRIS-1","state":"merged","url":"https://gitlab.example.com/acme/hris-api/-/merge_requests/12","source_branch":"feature/HRIS-1","updated_at":"2026-09-21 08:00:00 UTC"}}`)
	if code := e.deliver(repo.Id, map[string]string{"X-Gitlab-Event": "Merge Request Hook", "X-Gitlab-Token": "nope"}, mr); code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", code)
	}
	if code := e.deliver(repo.Id, map[string]string{"X-Gitlab-Event": "Merge Request Hook", "X-Gitlab-Token": *repo.Secret}, mr); code != http.StatusAccepted {
		t.Fatalf("delivery: %d", code)
	}
	e.processAll()
	var got httpapi.Ticket
	e.call(w.pm, http.MethodGet, "/tickets/"+tk.Key, nil, &got)
	if got.Code == nil || len(got.Code.MergeRequests) != 1 || got.Code.MergeRequests[0].Number != 12 || got.Code.MergeRequests[0].State != "merged" || got.Code.MergeRequests[0].MergedAt == nil {
		t.Fatalf("code: %+v", got.Code)
	}
}

// A delivery links and moves only tickets in its repository's project; a
// commit URL that is not http or https is stored empty.
func TestWebhookTouchesOnlyItsProject(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{7}, 32) })
	w := newHRIS(e)
	ops := e.seedProject("OPS")
	admin, _ := e.signedIn("admin@example.com", true)
	mine := e.seedTicket(w.p, w.pmUser, "Supervisor skip", &w.a, w.ot)
	other := e.seedTicket(ops, w.pmUser, "Disk full", nil)
	var repo httpapi.Repo
	if code := e.call(admin, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "github", "name": "hris-app", "web_url": "https://github.com/acme/hris-app"}, &repo); code != http.StatusCreated {
		t.Fatalf("repo: %d", code)
	}
	push := []byte(`{"ref":"refs/heads/main","commits":[{"id":"abc1234def","message":"Fixes HRIS-1 and OPS-1","url":"javascript:alert(1)","author":{"name":"PM","email":"pm@example.com"}}]}`)
	if code := e.deliver(repo.Id, map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": sign(*repo.Secret, push)}, push); code != http.StatusAccepted {
		t.Fatalf("delivery: %d", code)
	}
	e.processAll()
	var mineStatus, otherStatus string
	var mineLinks, otherLinks int
	q := `SELECT s.name, (SELECT count(*) FROM ticket_commits tc WHERE tc.ticket_id = t.id) FROM tickets t JOIN statuses s ON s.id = t.status_id WHERE t.id = $1`
	if err := e.d.Pool.QueryRow(t.Context(), q, mine.ID).Scan(&mineStatus, &mineLinks); err != nil || mineStatus != "In review" || mineLinks != 1 {
		t.Fatalf("own ticket: %v %q %d", err, mineStatus, mineLinks)
	}
	if err := e.d.Pool.QueryRow(t.Context(), q, other.ID).Scan(&otherStatus, &otherLinks); err != nil || otherStatus == "In review" || otherLinks != 0 {
		t.Fatalf("other project's ticket: %v %q %d", err, otherStatus, otherLinks)
	}
	var url *string
	if err := e.d.Pool.QueryRow(t.Context(), "SELECT url FROM commits WHERE sha = 'abc1234def'").Scan(&url); err != nil || (url != nil && *url != "") {
		t.Fatalf("commit url: %v %v", err, url)
	}
}

// An unknown repository answers like a bad signature; rejections are audited
// once a minute per repository; over 60 requests a minute from one address
// answers 429 and audits nothing.
func TestWebhookLimits(t *testing.T) {
	e := newEnvWith(t, func(c *config.Config) { c.SecretKey = bytes.Repeat([]byte{7}, 32) })
	newHRIS(e)
	admin, _ := e.signedIn("admin@example.com", true)
	var repo httpapi.Repo
	e.call(admin, http.MethodPost, "/projects/HRIS/repos", map[string]any{"provider": "github", "name": "hris-app", "web_url": "https://github.com/acme/hris-app"}, &repo)
	bad := map[string]string{"X-GitHub-Event": "push", "X-Hub-Signature-256": "sha256=00"}
	body := []byte(`{}`)
	rejected := func() int {
		var n int
		e.d.Pool.QueryRow(t.Context(), "SELECT count(*) FROM audit_events WHERE action = 'webhook_rejected'").Scan(&n)
		return n
	}
	for range 2 {
		if code := e.deliver(repo.Id, bad, body); code != http.StatusUnauthorized {
			t.Fatalf("bad signature: %d", code)
		}
	}
	if n := rejected(); n != 1 {
		t.Fatalf("rejections audited: %d", n)
	}
	for i := 3; i <= 60; i++ {
		if code := e.deliver(999999, bad, body); code != http.StatusUnauthorized {
			t.Fatalf("unknown repo, request %d: %d", i, code)
		}
	}
	if code := e.deliver(repo.Id, bad, body); code != http.StatusTooManyRequests {
		t.Fatalf("61st request: %d", code)
	}
	if n := rejected(); n != 1 {
		t.Fatalf("rejections audited after the limit: %d", n)
	}
}
