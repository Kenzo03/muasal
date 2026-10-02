package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/gitlink"
	"github.com/kenzo03/muasal/server/internal/secret"
)

// webhookMaxBytes is the largest delivery the webhook takes (§14.1).
const webhookMaxBytes = 5 << 20

// ListRepos lists a project's repositories for its admins.
func (s *Server) ListRepos(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	rows, err := s.q.ListRepos(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := RepoList{Items: make([]Repo, len(rows))}
	for i, repo := range rows {
		out.Items[i] = s.toAPIRepo(repo, "")
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateRepo adds a repository with a new secret, shown this once.
func (s *Server) CreateRepo(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in RepoInput
	if !decodeJSON(w, r, &in) {
		return
	}
	name, webURL, fields := checkRepo(in.Name, in.WebUrl)
	if !slices.Contains(gitlink.Providers, string(in.Provider)) {
		fields = append(fields, FieldError{Field: "provider", Code: "invalid", Message: "Choose GitHub, GitLab or Gitea"})
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	plain, sealed, ok := s.newRepoSecret(w)
	if !ok {
		return
	}
	ctx := r.Context()
	var repo db.GitRepo
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if repo, err = q.CreateRepo(ctx, db.CreateRepoParams{ProjectID: pc.project.ID, Provider: string(in.Provider), Name: name, WebUrl: webURL, SecretEnc: sealed}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "repo", repo.ID, "create", map[string]any{"name": name, "provider": in.Provider})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toAPIRepo(repo, plain))
}

// UpdateRepo renames a repository or replaces its secret.
func (s *Server) UpdateRepo(w http.ResponseWriter, r *http.Request, id int64) {
	pc, repo, ok := s.repoFor(w, r, id)
	if !ok {
		return
	}
	var in RepoUpdate
	if !decodeJSON(w, r, &in) {
		return
	}
	name, webURL, fields := checkRepo(in.Name, in.WebUrl)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var plain string
	var sealed []byte
	if deref(in.NewSecret) {
		if plain, sealed, ok = s.newRepoSecret(w); !ok {
			return
		}
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		var err error
		if repo, err = q.UpdateRepo(ctx, db.UpdateRepoParams{ID: id, Name: name, WebUrl: webURL, SecretEnc: sealed}); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "repo", id, "update", map[string]any{"name": name, "new_secret": plain != ""})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toAPIRepo(repo, plain))
}

// DeleteRepo removes a repository with its commits and merge requests.
func (s *Server) DeleteRepo(w http.ResponseWriter, r *http.Request, id int64) {
	pc, repo, ok := s.repoFor(w, r, id)
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteRepo(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "repo", id, "delete", map[string]any{"name": repo.Name})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// gitWebhook takes a delivery (§14.1): over 60 requests a minute from one
// address is 429, a body over 5 MB is 413, an unknown repository or a bad
// signature is 401, with at most one audit event per repository a minute. A
// good one is stored and answered 202 at once; a job does the work.
func (s *Server) gitWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.hookIP.Allow(clientIP(r)) {
		writeProblem(w, http.StatusTooManyRequests, "rate_limited", "Too many webhook requests from this address; wait a minute")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("repo_id"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "bad_signature", "The webhook signature does not match")
		return
	}
	ctx := r.Context()
	repo, err := s.q.GetRepo(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusUnauthorized, "bad_signature", "The webhook signature does not match")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookMaxBytes+1))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", "The body could not be read")
		return
	}
	if len(body) > webhookMaxBytes {
		writeProblem(w, http.StatusRequestEntityTooLarge, "too_large", "Deliveries take at most 5 MB")
		return
	}
	key, err := secret.Open(s.cfg.SecretKey, repo.SecretEnc)
	if err == nil {
		err = gitlink.Verify(repo.Provider, key, r.Header, body)
	}
	if err != nil {
		if s.hookRej.Allow(strconv.FormatInt(repo.ID, 10)) {
			_ = audit(ctx, s.q, auditMeta{via: "webhook", requestID: ptr(requestIDFrom(ctx)), ip: ipAddr(r), projectID: &repo.ProjectID},
				nil, "repo", repo.ID, "webhook_rejected", map[string]any{"reason": "bad signature"})
		}
		writeProblem(w, http.StatusUnauthorized, "bad_signature", "The webhook signature does not match")
		return
	}
	event := gitlink.Event(repo.Provider, r.Header)
	if event == "" {
		w.WriteHeader(http.StatusAccepted) // a ping or an event Muasal does not use
		return
	}
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		did, err := q.InsertDelivery(ctx, db.InsertDeliveryParams{RepoID: repo.ID, Event: event, Payload: body})
		if err != nil {
			return err
		}
		_, err = s.jobs.InsertTx(ctx, tx, gitlink.ProcessDelivery{DeliveryID: did}, nil)
		return err
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) repoFor(w http.ResponseWriter, r *http.Request, id int64) (projectCtx, db.GitRepo, bool) {
	repo, err := s.q.GetRepo(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Repository not found")
		return projectCtx{}, repo, false
	}
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, repo, false
	}
	p, err := s.q.GetProjectByID(r.Context(), repo.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return projectCtx{}, repo, false
	}
	pc, ok := s.projectFor(w, r, p.Key, access.Admin)
	return pc, repo, ok
}

// newRepoSecret makes a webhook secret and seals it under APP_SECRET_KEY.
func (s *Server) newRepoSecret(w http.ResponseWriter) (string, []byte, bool) {
	plain := rand.Text() + rand.Text()
	sealed, err := secret.Seal(s.cfg.SecretKey, []byte(plain))
	if err != nil {
		writeProblem(w, http.StatusConflict, "secret_key_missing", "Set APP_SECRET_KEY on the server before adding repositories")
		return "", nil, false
	}
	return plain, sealed, true
}

func checkRepo(name, webURL string) (string, string, []FieldError) {
	name, webURL = strings.TrimSpace(name), strings.TrimSpace(webURL)
	var f []FieldError
	if n := utf8.RuneCountInString(name); n < 1 || n > 200 {
		f = append(f, FieldError{Field: "name", Code: "invalid", Message: "Use 1 to 200 characters"})
	}
	if u, err := url.Parse(webURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		f = append(f, FieldError{Field: "web_url", Code: "invalid", Message: "Use the repository's http or https address"})
	}
	return name, webURL, f
}

func (s *Server) toAPIRepo(r db.GitRepo, plain string) Repo {
	out := Repo{Id: r.ID, Provider: RepoProvider(r.Provider), Name: r.Name, WebUrl: r.WebUrl, CreatedAt: r.CreatedAt,
		WebhookUrl: fmt.Sprintf("%s/webhooks/git/%d", strings.TrimRight(s.cfg.PublicURL, "/"), r.ID)}
	if plain != "" {
		out.Secret = &plain
	}
	return out
}

// codeOf is a ticket's Code section: merge requests, then commits (§14.1).
func codeOf(ctx context.Context, q *db.Queries, ticketID int64) (*TicketCode, error) {
	mrs, err := q.ListTicketMergeRequests(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	commits, err := q.ListTicketCommits(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if len(mrs)+len(commits) == 0 {
		return nil, nil
	}
	out := &TicketCode{MergeRequests: []CodeMergeRequest{}, Commits: []CodeCommit{}}
	for _, m := range mrs {
		out.MergeRequests = append(out.MergeRequests, CodeMergeRequest{Repo: m.RepoName, Number: int(m.Number), Title: m.Title, State: m.State, MergedAt: m.MergedAt, Url: m.Url})
	}
	for _, c := range commits {
		first, _, _ := strings.Cut(strings.TrimSpace(c.Message), "\n")
		out.Commits = append(out.Commits, CodeCommit{Repo: c.RepoName, Sha: c.Sha, Message: first, Author: c.AuthorName, CommittedAt: c.CommittedAt, Url: c.Url})
	}
	return out, nil
}
