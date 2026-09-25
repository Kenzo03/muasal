package ask_test

import (
	"context"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// world is HRIS with Client A and Client B, HR › Attendance › Overtime
// Approval, an admin, and a member scoped to Client A. Tickets are indexed and
// embedded through the real indexer against a fake model server.
type world struct {
	t      *testing.T
	d      testdb.DB
	q      *db.Queries
	rt     *ai.Runtime
	fake   *llmtest.Server
	p      db.Project
	a, b   db.Client
	ot     db.Node
	hr     db.Node
	admin  db.User
	member db.User
	done   int64
}

func newWorld(t *testing.T) *world {
	d := testdb.New(t)
	ctx := context.Background()
	w := &world{t: t, d: d, q: db.New(d.Pool), fake: llmtest.New(t)}
	w.rt = &ai.Runtime{Store: ai.NewStore(w.q), Gate: ai.NewGate()}
	w.admin = must(w.q.CreateUser(ctx, db.CreateUserParams{Email: "admin@example.com", Name: "Hana", Locale: "id", Timezone: "Asia/Jakarta", IsAdmin: true}))
	w.member = must(w.q.CreateUser(ctx, db.CreateUserParams{Email: "rina@example.com", Name: "Rina", Locale: "id", Timezone: "Asia/Jakarta"}))
	w.p = must(w.q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"}))
	w.a = must(w.q.CreateClient(ctx, db.CreateClientParams{Name: "Client A", Aliases: []string{"Arunika"}}))
	w.b = must(w.q.CreateClient(ctx, db.CreateClientParams{Name: "Client B", Aliases: []string{}}))
	w.check(w.q.LinkClients(ctx, db.LinkClientsParams{ProjectID: w.p.ID, ClientIds: []int64{w.a.ID, w.b.ID}}))
	w.check(w.q.UpsertMembership(ctx, db.UpsertMembershipParams{UserID: w.member.ID, ProjectID: w.p.ID, Role: "member"}))
	w.check(w.q.AddMembershipClients(ctx, db.AddMembershipClientsParams{UserID: w.member.ID, ProjectID: w.p.ID, ClientIds: []int64{w.a.ID}}))
	w.hr = must(w.q.CreateNode(ctx, db.CreateNodeParams{ProjectID: w.p.ID, Type: "module", Name: "HR", Aliases: []string{}}))
	att := must(w.q.CreateNode(ctx, db.CreateNodeParams{ProjectID: w.p.ID, ParentID: &w.hr.ID, Type: "module", Name: "Attendance", Aliases: []string{}}))
	w.ot = must(w.q.CreateNode(ctx, db.CreateNodeParams{ProjectID: w.p.ID, ParentID: &att.ID, Type: "menu", Name: "Overtime Approval", Aliases: []string{"approval lembur"}}))
	statuses := must(w.q.ListStatuses(ctx, w.p.ID))
	w.done = statuses[3].ID
	s := ai.Defaults()
	s.Mode, s.Chat.URL, s.Embed.URL = ai.ModeLocal, w.fake.BaseURL(), w.fake.BaseURL()
	w.check(w.rt.Store.Put(ctx, w.q, s, w.admin.ID))
	return w
}

func (w *world) check(err error) {
	w.t.Helper()
	if err != nil {
		w.t.Fatal(err)
	}
}

// must panics on a setup error, like the db package's tests.
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// ticket seeds and indexes one ticket; closed is its close day ("" leaves it
// open); decision is its confirmed what-changed ("" for none).
func (w *world) ticket(title string, client *db.Client, node db.Node, reason, closed, decision string) db.Ticket {
	w.t.Helper()
	ctx := context.Background()
	n := must(w.q.NextTicketNumber(ctx, w.p.ID))
	params := db.CreateTicketParams{
		ProjectID: w.p.ID, Number: n, Key: "HRIS-" + itoa(n), Type: "change_request", Title: title, Reason: reason,
		StatusID: must(w.q.ListStatuses(ctx, w.p.ID))[0].ID, RequesterUserID: &w.admin.ID, ReporterID: w.admin.ID, Priority: "medium",
	}
	if client != nil {
		params.ClientID = &client.ID
	}
	tk := must(w.q.CreateTicket(ctx, params))
	w.check(w.q.AddTicketNodes(ctx, db.AddTicketNodesParams{TicketID: tk.ID, NodeIds: []int64{node.ID}}))
	if closed != "" {
		at, _ := time.Parse(time.DateOnly, closed)
		_, err := w.d.Pool.Exec(ctx, "UPDATE tickets SET status_id = $2, closed_at = $3 WHERE id = $1", tk.ID, w.done, at.Add(3*time.Hour))
		w.check(err)
		if decision != "" {
			_, err := w.q.ConfirmDecision(ctx, db.ConfirmDecisionParams{TicketID: tk.ID, WhatChanged: decision, Why: reason, Outcome: "implemented", ConfirmedBy: w.admin.ID})
			w.check(err)
		}
	}
	ix := indexer.New(w.d.Pool, w.rt)
	w.check(ix.Rebuild(ctx, tk.ID))
	w.check(ix.EmbedTicket(ctx, tk.ID))
	return tk
}

func (w *world) asker(u db.User) ask.Asker {
	tz, _ := time.LoadLocation("Asia/Jakarta")
	return ask.Asker{UserID: u.ID, IsAdmin: u.IsAdmin, Locale: "en", TZ: tz}
}

func itoa(n int64) string {
	b := []byte{}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
