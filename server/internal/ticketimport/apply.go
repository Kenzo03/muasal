package ticketimport

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/db"
)

// Project is what an import resolves values against: the project's statuses,
// live nodes and clients.
type Project struct {
	ID       int64
	Key      string
	Statuses []db.Status
	Nodes    []db.ListNodesRow
	Clients  []db.Client
}

// LoadProject reads the project as an import sees it.
func LoadProject(ctx context.Context, q *db.Queries, id int64) (Project, error) {
	p, err := q.GetProjectByID(ctx, id)
	if err != nil {
		return Project{}, err
	}
	out := Project{ID: p.ID, Key: p.Key}
	if out.Statuses, err = q.ListStatuses(ctx, id); err != nil {
		return out, err
	}
	if out.Nodes, err = q.ListNodes(ctx, db.ListNodesParams{ProjectID: id, AllClients: true, ClientIds: []int64{}}); err != nil {
		return out, err
	}
	out.Clients, err = q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: id, AllClients: true, ClientIds: []int64{}})
	return out, err
}

// Resolved is a record's values in Zettra's terms.
type Resolved struct {
	Type, Priority string
	Status         db.Status
	ClientID       *int64
	NodeIDs        []int64
	Closed         bool
}

// Resolve converts a record's values; problems reject the row.
func (p Project) Resolve(r Record, m Mapping) (Resolved, []string) {
	var out Resolved
	var problems []string
	if r.Key == "" {
		problems = append(problems, "No key: a re-import could not find this ticket again")
	}
	if strings.TrimSpace(r.Title) == "" {
		problems = append(problems, "No title")
	} else if n := len([]rune(r.Title)); n > 200 {
		problems = append(problems, "The title is longer than 200 characters")
	}
	out.Type = "change_request"
	if r.Type != "" {
		t, ok := m.Types[strings.ToLower(r.Type)]
		if !ok || !slices.Contains([]string{"bug", "change_request", "feature"}, t) {
			problems = append(problems, fmt.Sprintf("Unknown type %q: map it to bug, change request or feature", r.Type))
		}
		out.Type = t
	}
	out.Priority = "medium"
	if pr, ok := m.Priorities[strings.ToLower(r.Priority)]; ok {
		out.Priority = pr
	}
	// The status maps by the mapping, else by name; a resolved row without one is Done, others the default.
	name := r.Status
	if to, ok := m.Statuses[strings.ToLower(r.Status)]; ok {
		name = to
	}
	found := false
	for _, s := range p.Statuses {
		if strings.EqualFold(s.Name, name) {
			out.Status, found = s, true
		}
	}
	if !found {
		for _, s := range p.Statuses {
			if (r.Resolved != nil && s.Category == "done") || (r.Resolved == nil && s.IsDefault) {
				out.Status, found = s, true
				break
			}
		}
	}
	out.Closed = out.Status.Category == "done" || out.Status.Category == "cancelled"
	if r.Client != "" {
		for _, c := range p.Clients {
			if strings.EqualFold(c.Name, r.Client) || strings.EqualFold(deref(c.Code), r.Client) ||
				slices.ContainsFunc(c.Aliases, func(a string) bool { return strings.EqualFold(a, r.Client) }) {
				out.ClientID = &c.ID
			}
		}
		if out.ClientID == nil {
			problems = append(problems, fmt.Sprintf("Unknown client %q: link it to the project first", r.Client))
		}
	}
	// Components and labels become menus: by the mapping, else by name, alias or code.
	for _, v := range append(slices.Clone(r.Components), r.Labels...) {
		if id, ok := m.Nodes[strings.ToLower(v)]; ok {
			if !slices.Contains(out.NodeIDs, id) {
				out.NodeIDs = append(out.NodeIDs, id)
			}
			continue
		}
		for _, n := range p.Nodes {
			if strings.EqualFold(n.Name, v) || strings.EqualFold(deref(n.Code), v) || slices.ContainsFunc(n.Aliases, func(a string) bool { return strings.EqualFold(a, v) }) {
				if !slices.Contains(out.NodeIDs, n.ID) {
					out.NodeIDs = append(out.NodeIDs, n.ID)
				}
				break
			}
		}
	}
	return out, problems
}

// Problem is a rejected row.
type Problem struct {
	Line    int    `json:"line"`
	Key     string `json:"key,omitempty"`
	Message string `json:"message"`
}

// Stats are a dry run's counts and, while the import runs, its progress.
type Stats struct {
	Rows     int `json:"rows"`
	Create   int `json:"create"`
	Update   int `json:"update"`
	Reject   int `json:"reject"`
	Linked   int `json:"linked"`   // valid rows with at least one menu
	Coverage int `json:"coverage"` // percent of valid rows mapped to a menu
	Done     int `json:"done"`     // rows written so far
	LastLine int `json:"last_line"`
	Tickets  int `json:"tickets"`  // created or updated so far
	Comments int `json:"comments"` // new comments so far
}

// Plan reads the whole file as a dry run: counts to create, update and
// reject, the first 20 errors, and module coverage (§14.2).
func Plan(ctx context.Context, q *db.Queries, p Project, r io.Reader, m Mapping) (Stats, []Problem, error) {
	refs, err := q.ListExternalRefs(ctx, p.ID)
	if err != nil {
		return Stats{}, nil, err
	}
	existing := map[string]bool{}
	for _, k := range refs {
		existing[k] = true
	}
	var st Stats
	var problems []Problem
	seen := map[string]int{}
	err = Read(r, m, func(rec Record) error {
		st.Rows++
		res, bad := p.Resolve(rec, m)
		if prev, dup := seen[rec.Key]; dup && rec.Key != "" {
			bad = append(bad, fmt.Sprintf("The key %s is also on row %d", rec.Key, prev))
		}
		if len(bad) > 0 {
			st.Reject++
			if len(problems) < 20 {
				problems = append(problems, Problem{Line: rec.Line, Key: rec.Key, Message: strings.Join(bad, "; ")})
			}
			return nil
		}
		seen[rec.Key] = rec.Line
		if existing[rec.Key] {
			st.Update++
		} else {
			st.Create++
		}
		if len(res.NodeIDs) > 0 {
			st.Linked++
		}
		return nil
	})
	if valid := st.Create + st.Update; valid > 0 {
		st.Coverage = st.Linked * 100 / valid
	}
	return st, problems, err
}

// Writer writes records to one project as one importing user.
type Writer struct {
	Project  Project
	Mapping  Mapping
	Importer int64
}

// Written is what one record changed, for the index and the history.
type Written struct {
	TicketID int64
	Created  bool
	Comments int
}

// Write creates or updates one record's ticket with its menus and comments in
// q's transaction. A rejected row returns nil and no error: the dry run
// already reported it.
func (wr Writer) Write(ctx context.Context, q *db.Queries, rec Record) (*Written, error) {
	res, bad := wr.Project.Resolve(rec, wr.Mapping)
	if len(bad) > 0 {
		return nil, nil
	}
	meta, _ := json.Marshal(rec.Raw)
	assignee := wr.user(ctx, q, rec.Assignee)
	created := time.Now()
	if rec.Created != nil {
		created = *rec.Created
	}
	var closed *time.Time
	if res.Closed {
		closed = rec.Resolved
		if closed == nil {
			closed = &created
		}
	}
	out := &Written{}
	t, err := q.GetTicketByExternalRef(ctx, db.GetTicketByExternalRefParams{ProjectID: wr.Project.ID, Ref: rec.Key})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		n, err := q.NextTicketNumber(ctx, wr.Project.ID)
		if err != nil {
			return nil, err
		}
		p := db.CreateImportedTicketParams{
			ProjectID: wr.Project.ID, Number: n, Key: fmt.Sprintf("%s-%d", wr.Project.Key, n), Type: res.Type, Title: strings.TrimSpace(rec.Title),
			Description: rec.Description, Reason: strings.TrimSpace(rec.Reason), StatusID: res.Status.ID, ClientID: res.ClientID,
			ReporterID: wr.Importer, AssigneeID: assignee, Priority: res.Priority, ExternalRef: &rec.Key, ExternalMeta: meta,
			CreatedAt: created, ClosedAt: closed,
		}
		if id := wr.user(ctx, q, rec.Reporter); id != nil {
			p.ReporterID, p.RequesterUserID = *id, id
		} else if rec.Reporter != "" {
			// People without an account become internal contacts (R-IN-4).
			cid, err := wr.contact(ctx, q, rec.Reporter)
			if err != nil {
				return nil, err
			}
			p.RequesterContactID = &cid
		} else {
			p.RequesterUserID = &wr.Importer
		}
		if t, err = q.CreateImportedTicket(ctx, p); err != nil {
			return nil, err
		}
		out.Created = true
	case err != nil:
		return nil, err
	default:
		if t, err = q.UpdateImportedTicket(ctx, db.UpdateImportedTicketParams{
			ID: t.ID, Type: res.Type, Title: strings.TrimSpace(rec.Title), Description: rec.Description, StatusID: res.Status.ID,
			Priority: res.Priority, AssigneeID: assignee, ExternalMeta: meta, ClosedAt: closed,
		}); err != nil {
			return nil, err
		}
	}
	out.TicketID = t.ID
	if len(res.NodeIDs) > 0 {
		if err := q.LinkImportedNodes(ctx, db.LinkImportedNodesParams{TicketID: t.ID, NodeIds: res.NodeIDs}); err != nil {
			return nil, err
		}
	}
	comments := slices.Clone(rec.Comments)
	if len(rec.Attachments) > 0 {
		// Jira's CSV carries attachment links only; they stay in a comment (R-IN-5).
		comments = append(comments, Comment{At: created, Body: "Original attachments: " + strings.Join(rec.Attachments, ", ")})
	}
	for _, c := range comments {
		if strings.TrimSpace(c.Body) == "" {
			continue
		}
		at := c.At
		if at.IsZero() {
			at = created
		}
		sum := sha256.Sum256([]byte(rec.Key + "\x00" + at.UTC().Format(time.RFC3339) + "\x00" + c.Author + "\x00" + c.Body))
		p := db.InsertImportedCommentParams{TicketID: t.ID, Body: c.Body, CreatedAt: at, ExternalHash: sum[:]}
		if id := wr.user(ctx, q, c.Author); id != nil {
			p.AuthorID = id
		} else {
			label := c.Author
			if label == "" {
				label = "Import"
			}
			p.AuthorLabel = &label
		}
		n, err := q.InsertImportedComment(ctx, p)
		if err != nil {
			return nil, err
		}
		out.Comments += int(n)
	}
	return out, nil
}

// user finds an account by name or email; none for an unknown person.
func (wr Writer) user(ctx context.Context, q *db.Queries, who string) *int64 {
	if strings.TrimSpace(who) == "" {
		return nil
	}
	id, err := q.FindUserByNameOrEmail(ctx, strings.TrimSpace(who))
	if err != nil {
		return nil
	}
	return &id
}

// contact finds or creates an internal contact with the name.
func (wr Writer) contact(ctx context.Context, q *db.Queries, name string) (int64, error) {
	id, err := q.FindInternalContact(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return q.CreateContact(ctx, db.CreateContactParams{Name: name})
	}
	return id, err
}

func deref[T any](p *T) T {
	var zero T
	if p != nil {
		return *p
	}
	return zero
}
