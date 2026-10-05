package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenzo03/zettra/server/internal/ai"
	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/indexer"
)

// DemoKey is the demo dataset's project.
const DemoKey = "DEMO"

// Dataset is a demo project: clients, contacts, the module tree and tickets
// with their decision records and comments. Dates are YYYY-MM-DD.
type Dataset struct {
	Project struct {
		Key, Name string
	} `json:"project"`
	Clients []struct {
		Name    string   `json:"name"`
		Code    string   `json:"code"`
		Aliases []string `json:"aliases"`
	} `json:"clients"`
	Contacts []struct {
		Name   string `json:"name"`
		Title  string `json:"title"`
		Client string `json:"client"` // "" for an internal person
	} `json:"contacts"`
	Users []struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"users"`
	Tree []struct {
		Path    string   `json:"path"` // "HR › Attendance › Overtime Approval"
		Type    string   `json:"type"`
		Aliases []string `json:"aliases"`
	} `json:"tree"`
	Tickets []struct {
		Type        string   `json:"type"`
		Title       string   `json:"title"`
		Client      string   `json:"client"` // "" for core work
		Menus       []string `json:"menus"`
		RequestedBy string   `json:"requested_by"` // a contact
		Reporter    string   `json:"reporter"`     // a user
		Reason      string   `json:"reason"`
		Description string   `json:"description"`
		Status      string   `json:"status"` // To do, In progress, In review, Done, Cancelled
		Created     string   `json:"created"`
		Closed      string   `json:"closed"`
		Decision    *struct {
			WhatChanged  string `json:"what_changed"`
			Why          string `json:"why"`
			Alternatives string `json:"alternatives"`
		} `json:"decision"`
		Comments []struct {
			By       string `json:"by"`
			On       string `json:"on"`
			Text     string `json:"text"`
			Internal bool   `json:"internal"`
		} `json:"comments"`
	} `json:"tickets"`
}

// LoadDataset reads a dataset file; "" reads the embedded demo.
func LoadDataset(path string) (Dataset, error) {
	var ds Dataset
	raw, err := read(path, "data/demo-hris.json")
	if err != nil {
		return ds, err
	}
	return ds, json.Unmarshal(raw, &ds)
}

// Seed loads the dataset once: a project that already exists is left as it
// is. Then it indexes and embeds every ticket of the project, so a run after
// a model change starts from a current index.
func Seed(ctx context.Context, pool *pgxpool.Pool, rt *ai.Runtime, ds Dataset, progress io.Writer) error {
	q := db.New(pool)
	p, err := q.GetProjectByKey(ctx, ds.Project.Key)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := load(ctx, pool, ds); err != nil {
			return fmt.Errorf("load %s: %w", ds.Project.Key, err)
		}
		p, err = q.GetProjectByKey(ctx, ds.Project.Key)
	}
	if err != nil {
		return err
	}
	ids, err := q.ListProjectTicketIDs(ctx, p.ID)
	if err != nil {
		return err
	}
	ix := indexer.New(pool, rt)
	for i, id := range ids {
		if err := ix.Rebuild(ctx, id); err != nil {
			return err
		}
		if err := ix.EmbedTicket(ctx, id); err != nil {
			return err
		}
		if progress != nil && (i+1)%10 == 0 {
			fmt.Fprintf(progress, "indexed %d of %d tickets\n", i+1, len(ids))
		}
	}
	return nil
}

// load writes the dataset in one transaction.
func load(ctx context.Context, pool *pgxpool.Pool, ds Dataset) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	p, err := q.CreateProject(ctx, db.CreateProjectParams{Key: ds.Project.Key, Name: ds.Project.Name})
	if err != nil {
		return err
	}
	clients := map[string]int64{}
	var clientIDs []int64
	for _, c := range ds.Clients {
		row, err := q.GetClientByName(ctx, c.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.CreateClient(ctx, db.CreateClientParams{Name: c.Name, Code: nonEmpty(c.Code), Aliases: c.Aliases})
		}
		if err != nil {
			return err
		}
		clients[c.Name] = row.ID
		clientIDs = append(clientIDs, row.ID)
	}
	if err := q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: clientIDs}); err != nil {
		return err
	}
	contacts := map[string]int64{}
	for _, c := range ds.Contacts {
		var client *int64
		if c.Client != "" {
			id := clients[c.Client]
			client = &id
		}
		id, err := q.CreateContact(ctx, db.CreateContactParams{ClientID: client, Name: c.Name, Title: nonEmpty(c.Title)})
		if err != nil {
			return err
		}
		contacts[c.Name] = id
	}
	users := map[string]int64{}
	for _, u := range ds.Users {
		row, err := q.GetUserByEmail(ctx, u.Email)
		if errors.Is(err, pgx.ErrNoRows) {
			row, err = q.CreateUser(ctx, db.CreateUserParams{Email: u.Email, Name: u.Name, Locale: "id", Timezone: "Asia/Jakarta"})
		}
		if err != nil {
			return err
		}
		users[u.Name] = row.ID
		if err := q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: row.ID, ProjectID: p.ID, Role: "member", AllClients: true}); err != nil {
			return err
		}
	}
	nodes := map[string]int64{}
	for _, n := range ds.Tree {
		parts := splitPath(n.Path)
		var parent *int64
		if len(parts) > 1 {
			id, ok := nodes[strings.Join(parts[:len(parts)-1], " › ")]
			if !ok {
				return fmt.Errorf("tree: %q comes before its parent", n.Path)
			}
			parent = &id
		}
		row, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, ParentID: parent, Type: n.Type, Name: parts[len(parts)-1], Aliases: orEmpty(n.Aliases)})
		if err != nil {
			return err
		}
		nodes[strings.Join(parts, " › ")] = row.ID
	}
	statuses, err := q.ListStatuses(ctx, p.ID)
	if err != nil {
		return err
	}
	statusID := map[string]int64{}
	for _, s := range statuses {
		statusID[s.Name] = s.ID
	}
	for i, t := range ds.Tickets {
		n, err := q.NextTicketNumber(ctx, p.ID)
		if err != nil {
			return err
		}
		reporter, ok := users[t.Reporter]
		contact, okc := contacts[t.RequestedBy]
		status, oks := statusID[t.Status]
		if !ok || !okc || !oks {
			return fmt.Errorf("ticket %d: unknown reporter %q, requester %q or status %q", i+1, t.Reporter, t.RequestedBy, t.Status)
		}
		params := db.CreateTicketParams{
			ProjectID: p.ID, Number: n, Key: fmt.Sprintf("%s-%d", p.Key, n), Type: t.Type, Title: t.Title,
			Description: t.Description, Reason: t.Reason, StatusID: status, RequesterContactID: &contact,
			ReporterID: reporter, Priority: "medium",
		}
		if t.Client != "" {
			id := clients[t.Client]
			params.ClientID = &id
		}
		tk, err := q.CreateTicket(ctx, params)
		if err != nil {
			return fmt.Errorf("ticket %d: %w", i+1, err)
		}
		var menus []int64
		for _, m := range t.Menus {
			id, ok := nodes[strings.Join(splitPath(m), " › ")]
			if !ok {
				return fmt.Errorf("ticket %d: unknown menu %q", i+1, m)
			}
			menus = append(menus, id)
		}
		if err := q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: tk.ID, NodeIds: menus}); err != nil {
			return err
		}
		created, _ := time.Parse(time.DateOnly, t.Created)
		var closed *time.Time
		if t.Closed != "" {
			c, _ := time.Parse(time.DateOnly, t.Closed)
			c = c.Add(10 * time.Hour)
			closed = &c
		}
		if _, err := tx.Exec(ctx, "UPDATE tickets SET created_at = $2, updated_at = $2, closed_at = $3 WHERE id = $1", tk.ID, created.Add(9*time.Hour), closed); err != nil {
			return err
		}
		if d := t.Decision; d != nil && closed != nil {
			outcome := "implemented"
			if t.Status == "Cancelled" {
				outcome = "rejected"
			}
			if _, err := q.ConfirmDecision(ctx, db.ConfirmDecisionParams{TicketID: tk.ID, WhatChanged: d.WhatChanged, Why: d.Why,
				Alternatives: d.Alternatives, Outcome: outcome, ConfirmedBy: reporter}); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE decision_records SET confirmed_at = $2 WHERE ticket_id = $1", tk.ID, *closed); err != nil {
				return err
			}
		}
		for _, c := range t.Comments {
			author, ok := users[c.By]
			if !ok {
				return fmt.Errorf("ticket %d: unknown comment author %q", i+1, c.By)
			}
			on, _ := time.Parse(time.DateOnly, c.On)
			if _, err := tx.Exec(ctx, "INSERT INTO comments (ticket_id, author_id, internal, body, created_at) VALUES ($1, $2, $3, $4, $5)",
				tk.ID, author, c.Internal, c.Text, on.Add(11*time.Hour)); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
