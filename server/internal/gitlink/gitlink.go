// Package gitlink links commits and merge requests to tickets from Git
// webhooks (FSD §14.1): it checks a delivery's signature, reads GitHub, GitLab
// and Gitea payloads, finds ticket keys and stores what they name.
package gitlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Providers the webhook understands.
var Providers = []string{"github", "gitlab", "gitea"}

// ErrSignature means the delivery is not from the repository's Git server.
var ErrSignature = errors.New("bad webhook signature")

// Verify checks a delivery against the repository's secret (§14.1):
// GitHub's X-Hub-Signature-256, GitLab's X-Gitlab-Token and Gitea's
// X-Gitea-Signature, each compared in constant time.
func Verify(provider string, secret []byte, h http.Header, body []byte) error {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sum := hex.EncodeToString(mac.Sum(nil))
	var ok bool
	switch provider {
	case "github":
		ok = hmac.Equal([]byte(h.Get("X-Hub-Signature-256")), []byte("sha256="+sum))
	case "gitea":
		ok = hmac.Equal([]byte(strings.ToLower(h.Get("X-Gitea-Signature"))), []byte(sum))
	case "gitlab":
		ok = subtle.ConstantTimeCompare([]byte(h.Get("X-Gitlab-Token")), secret) == 1
	}
	if !ok {
		return ErrSignature
	}
	return nil
}

// Event names the delivery's kind as "push", "merge_request" or "" for others.
func Event(provider string, h http.Header) string {
	var e string
	switch provider {
	case "github":
		e = h.Get("X-GitHub-Event")
	case "gitea":
		e = h.Get("X-Gitea-Event")
	case "gitlab":
		e = h.Get("X-Gitlab-Event")
	}
	switch e {
	case "push", "Push Hook":
		return "push"
	case "pull_request", "Merge Request Hook":
		return "merge_request"
	}
	return ""
}

// Commit is one pushed commit.
type Commit struct {
	SHA, Message, AuthorName, AuthorEmail, URL string
	At                                         *time.Time
}

// MergeRequest is a pull or merge request's current state.
type MergeRequest struct {
	Number                  int
	Title, Body, State, URL string
	Branch                  string
	MergedAt                *time.Time
}

// Parse reads a push or merge-request payload; branch is the pushed branch.
func Parse(provider, event string, payload []byte) (commits []Commit, mr *MergeRequest, branch string, err error) {
	switch event {
	case "push":
		var p struct {
			Ref     string `json:"ref"`
			Commits []struct {
				ID        string     `json:"id"`
				Message   string     `json:"message"`
				Timestamp *time.Time `json:"timestamp"`
				URL       string     `json:"url"`
				Author    struct {
					Name  string `json:"name"`
					Email string `json:"email"`
				} `json:"author"`
			} `json:"commits"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, "", err
		}
		for _, c := range p.Commits {
			commits = append(commits, Commit{SHA: c.ID, Message: c.Message, AuthorName: c.Author.Name, AuthorEmail: c.Author.Email, URL: webURL(c.URL), At: c.Timestamp})
		}
		return commits, nil, strings.TrimPrefix(p.Ref, "refs/heads/"), nil
	case "merge_request":
		if provider == "gitlab" {
			var p struct {
				Attrs struct {
					IID          int    `json:"iid"`
					Title        string `json:"title"`
					Description  string `json:"description"`
					State        string `json:"state"`
					URL          string `json:"url"`
					SourceBranch string `json:"source_branch"`
					UpdatedAt    string `json:"updated_at"`
				} `json:"object_attributes"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, nil, "", err
			}
			a := p.Attrs
			m := &MergeRequest{Number: a.IID, Title: a.Title, Body: a.Description, State: a.State, URL: webURL(a.URL), Branch: a.SourceBranch}
			if a.State == "merged" {
				if t, err := time.Parse("2006-01-02 15:04:05 MST", a.UpdatedAt); err == nil {
					m.MergedAt = &t
				} else if t, err := time.Parse(time.RFC3339, a.UpdatedAt); err == nil {
					m.MergedAt = &t
				}
			}
			return nil, m, "", nil
		}
		var p struct {
			PR struct {
				Number   int        `json:"number"`
				Title    string     `json:"title"`
				Body     string     `json:"body"`
				State    string     `json:"state"`
				Merged   bool       `json:"merged"`
				MergedAt *time.Time `json:"merged_at"`
				HTMLURL  string     `json:"html_url"`
				Head     struct {
					Ref string `json:"ref"`
				} `json:"head"`
			} `json:"pull_request"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, "", err
		}
		pr := p.PR
		m := &MergeRequest{Number: pr.Number, Title: pr.Title, Body: pr.Body, State: pr.State, URL: webURL(pr.HTMLURL), Branch: pr.Head.Ref, MergedAt: pr.MergedAt}
		if pr.Merged {
			m.State = "merged"
		}
		return nil, m, "", nil
	}
	return nil, nil, "", nil
}

// webURL returns s when it is an http or https address, else "".
func webURL(s string) string {
	if u, err := url.Parse(s); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return s
	}
	return ""
}

// keyRe is §14.1's ticket key pattern.
var keyRe = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})-([0-9]{1,7})\b`)

// fixRe finds "Fixes DMS-2", "closes: DMS-2, DMS-3" and the like (MSL-29).
var fixRe = regexp.MustCompile(`(?i:\b(?:fix(?:e[sd])?|close[sd]?|resolve[sd]?))\b:?\s+((?:[A-Z][A-Z0-9]{1,9}-[0-9]{1,7}\b(?:\s*,\s*|\s+and\s+|\s+dan\s+)?)+)`)

// Fixes lists the ticket keys a commit message says it fixes, closes or
// resolves, in order.
func Fixes(message string) []string {
	var out []string
	for _, m := range fixRe.FindAllStringSubmatch(message, -1) {
		for _, k := range keyRe.FindAllString(m[1], -1) {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// Keys lists the distinct ticket keys in the texts, in order.
func Keys(texts ...string) []string {
	var out []string
	for _, t := range texts {
		for _, k := range keyRe.FindAllString(t, -1) {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}
