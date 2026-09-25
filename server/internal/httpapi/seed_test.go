package httpapi_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/httpapi"
)

// The seeders write straight through the queries, so a test depends only on
// the handlers it exercises.

// signedIn seeds a user with the shared test password and signs them in.
func (e *env) signedIn(email string, admin bool) (*http.Client, db.User) {
	e.t.Helper()
	u := e.seedUser(email, pw, admin)
	c := e.client()
	if code, _ := login(e, c, email, pw); code != http.StatusOK {
		e.t.Fatalf("sign in %s: %d", email, code)
	}
	return c, u
}

func (e *env) seedProject(key string, clients ...db.Client) db.Project {
	e.t.Helper()
	ctx := context.Background()
	p, err := e.q.CreateProject(ctx, db.CreateProjectParams{Key: key, Name: key})
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.q.LinkClients(ctx, db.LinkClientsParams{ProjectID: p.ID, ClientIds: clientIDs(clients)}); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) seedClient(name string) db.Client {
	e.t.Helper()
	c, err := e.q.CreateClient(context.Background(), db.CreateClientParams{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// seedMember adds u to p with role; without clients the membership covers all clients.
func (e *env) seedMember(u db.User, p db.Project, role string, clients ...db.Client) {
	e.t.Helper()
	ctx := context.Background()
	err := e.q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: u.ID, ProjectID: p.ID, Role: role, AllClients: len(clients) == 0})
	if err == nil {
		err = e.q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: u.ID, ProjectID: p.ID, ClientIds: clientIDs(clients)})
	}
	if err != nil {
		e.t.Fatal(err)
	}
}

// seedNode adds a node under parent (nil = top level); clients make it a client-specific menu.
func (e *env) seedNode(p db.Project, parent *db.Node, typ, name string, clients ...db.Client) db.Node {
	e.t.Helper()
	ctx := context.Background()
	params := db.CreateNodeParams{ProjectID: p.ID, Type: typ, Name: name, ClientSpecific: len(clients) > 0}
	if parent != nil {
		params.ParentID = &parent.ID
	}
	n, err := e.q.CreateNode(ctx, params)
	if err == nil {
		err = e.q.AddNodeClients(ctx, db.AddNodeClientsParams{NodeID: n.ID, ProjectID: p.ID, ClientIds: clientIDs(clients)})
	}
	if err != nil {
		e.t.Fatal(err)
	}
	return n
}

// seedContact adds a contact of client, or an internal one when client is nil.
func (e *env) seedContact(name string, client *db.Client) int64 {
	e.t.Helper()
	params := db.CreateContactParams{Name: name}
	if client != nil {
		params.ClientID = &client.ID
	}
	id, err := e.q.CreateContact(context.Background(), params)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func clientIDs(clients []db.Client) []int64 {
	ids := make([]int64, len(clients))
	for i, c := range clients {
		ids[i] = c.ID
	}
	return ids
}

// firstError returns a problem's first field error, or a zero one.
func firstError(p httpapi.Problem) httpapi.FieldError {
	if p.Errors == nil || len(*p.Errors) == 0 {
		return httpapi.FieldError{}
	}
	return (*p.Errors)[0]
}

// seedTicket files a ticket in the project's default status, requested by its reporter.
func (e *env) seedTicket(p db.Project, reporter db.User, title string, client *db.Client, nodes ...db.Node) db.Ticket {
	e.t.Helper()
	ctx := context.Background()
	status, err := e.q.GetDefaultStatus(ctx, p.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	n, err := e.q.NextTicketNumber(ctx, p.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	params := db.CreateTicketParams{
		ProjectID: p.ID, Number: n, Key: fmt.Sprintf("%s-%d", p.Key, n), Type: "change_request", Title: title,
		StatusID: status.ID, RequesterUserID: &reporter.ID, ReporterID: reporter.ID, Priority: "medium",
	}
	if client != nil {
		params.ClientID = &client.ID
	}
	t, err := e.q.CreateTicket(ctx, params)
	if err != nil {
		e.t.Fatal(err)
	}
	ids := make([]int64, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	if err := e.q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: t.ID, NodeIds: ids}); err != nil {
		e.t.Fatal(err)
	}
	return t
}

// seedClose closes a ticket in status with a decision record confirmed by by;
// a Cancelled status records a rejection (R-DC-2).
func (e *env) seedClose(tk db.Ticket, status int64, by db.User, whatChanged string) {
	e.t.Helper()
	ctx := context.Background()
	st, err := e.q.GetStatus(ctx, status)
	if err == nil {
		_, err = e.q.SetTicketStatus(ctx, db.SetTicketStatusParams{ID: tk.ID, StatusID: status, Closed: true})
	}
	outcome := "implemented"
	if st.Category == "cancelled" {
		outcome = "rejected"
	}
	if err == nil {
		_, err = e.q.ConfirmDecision(ctx, db.ConfirmDecisionParams{
			TicketID: tk.ID, WhatChanged: whatChanged, Why: "Because the client asked for it this way.", Outcome: outcome, ConfirmedBy: by.ID,
		})
	}
	if err != nil {
		e.t.Fatal(err)
	}
}
