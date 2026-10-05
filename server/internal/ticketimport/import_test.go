package ticketimport_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/db"
	"github.com/kenzo03/zettra/server/internal/testdb"
	"github.com/kenzo03/zettra/server/internal/ticketimport"
)

// A Jira "Export Excel CSV (all fields)" in miniature: repeated Comment,
// Labels and Attachment columns, Jira's date format, one bad row.
const jiraCSV = `Summary,Issue key,Issue Type,Status,Priority,Reporter,Assignee,Created,Resolved,Component/s,Labels,Labels,Description,Comment,Comment,Attachment
Payroll export in BCA format,PAY-332,Story,Done,High,Rina,Budi Santoso,12/Mar/24 2:05 PM,20/Mar/24 9:00 AM,Payroll,bank,export,Client asked for KlikBCA.,14/Mar/24 10:00 AM;rina@example.com;Format confirmed with the bank.,15/Mar/24 11:30 AM;Agus;Tested on staging.,https://jira.example.com/secure/attachment/1/sample.csv
Overtime rounding,PAY-333,Improvement,In Progress,Medium,Agus,,13/Mar/24 9:00 AM,,Nothing Here,,,Round to 15 minutes.,,,
Broken row,PAY-334,Spike,To Do,Low,Agus,,13/Mar/24 9:00 AM,,,,,,,,
`

func TestJiraImportIsIdempotent(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	rina, err := q.CreateUser(ctx, db.CreateUserParams{Email: "rina@example.com", Name: "Rina", Locale: "id", Timezone: "Asia/Jakarta"})
	check(t, err)
	admin, err := q.CreateUser(ctx, db.CreateUserParams{Email: "admin@example.com", Name: "Hana", Locale: "id", Timezone: "Asia/Jakarta", IsAdmin: true})
	check(t, err)
	p, err := q.CreateProject(ctx, db.CreateProjectParams{Key: "HRIS", Name: "HRIS"})
	check(t, err)
	payroll, err := q.CreateNode(ctx, db.CreateNodeParams{ProjectID: p.ID, Type: "module", Name: "Payroll", Aliases: []string{}})
	check(t, err)

	file := filepath.Join(t.TempDir(), "jira.csv")
	check(t, os.WriteFile(file, []byte(jiraCSV), 0o600))
	m := ticketimport.JiraPreset()
	proj, err := ticketimport.LoadProject(ctx, q, p.ID)
	check(t, err)
	f, _ := os.Open(file)
	st, problems, err := ticketimport.Plan(ctx, q, proj, f, m)
	f.Close()
	check(t, err)
	if st.Rows != 3 || st.Create != 2 || st.Reject != 1 || st.Coverage != 50 || len(problems) != 1 || problems[0].Line != 4 ||
		!strings.Contains(problems[0].Message, `Unknown type "Spike"`) {
		t.Fatalf("dry run: %+v %+v", st, problems)
	}

	mapping, _ := json.Marshal(m)
	run, err := q.CreateImportRun(ctx, db.CreateImportRunParams{ProjectID: p.ID, FileName: "jira.csv", FilePath: file, Mapping: mapping, CreatedBy: admin.ID})
	check(t, err)
	noIndex := func(context.Context, pgx.Tx, []int64) error { return nil }
	check(t, ticketimport.Run(ctx, d.Pool, run.ID, noIndex))

	var tickets, comments, contacts int
	count := func() {
		check(t, d.Pool.QueryRow(ctx, "SELECT count(*) FROM tickets").Scan(&tickets))
		check(t, d.Pool.QueryRow(ctx, "SELECT count(*) FROM comments").Scan(&comments))
		check(t, d.Pool.QueryRow(ctx, "SELECT count(*) FROM contacts").Scan(&contacts))
	}
	count()
	if tickets != 2 || comments != 3 || contacts != 1 { // two Jira comments plus the attachments note; Agus becomes a contact
		t.Fatalf("after one import: %d tickets, %d comments, %d contacts", tickets, comments, contacts)
	}
	tk, err := q.GetTicketByExternalRef(ctx, db.GetTicketByExternalRefParams{ProjectID: p.ID, Ref: "pay-332"})
	check(t, err)
	if tk.Key != "HRIS-1" || tk.Type != "feature" || tk.Priority != "high" || tk.Source != "import" || tk.ClosedAt == nil ||
		tk.RequesterUserID == nil || *tk.RequesterUserID != rina.ID || tk.CreatedAt.Format("2006-01-02") != "2024-03-12" {
		t.Fatalf("PAY-332: %+v", tk)
	}
	var author *int64
	var label *string
	check(t, d.Pool.QueryRow(ctx, "SELECT author_id, author_label FROM comments WHERE body = 'Tested on staging.'").Scan(&author, &label))
	if author != nil || label == nil || *label != "Agus" {
		t.Fatalf("an author without an account: %v %v", author, label)
	}
	var onPayroll bool
	check(t, d.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM ticket_nodes WHERE ticket_id = $1 AND node_id = $2)", tk.ID, payroll.ID).Scan(&onPayroll))
	if !onPayroll {
		t.Fatal("Component/s Payroll did not link the Payroll node")
	}

	// AC-IN-3: the same file again changes no counts.
	again, err := q.CreateImportRun(ctx, db.CreateImportRunParams{ProjectID: p.ID, FileName: "jira.csv", FilePath: file, Mapping: mapping, CreatedBy: admin.ID})
	check(t, err)
	check(t, ticketimport.Run(ctx, d.Pool, again.ID, noIndex))
	before := [3]int{tickets, comments, contacts}
	count()
	if [3]int{tickets, comments, contacts} != before {
		t.Fatalf("second import: %d tickets, %d comments, %d contacts; before %v", tickets, comments, contacts, before)
	}
	r, err := q.GetImportRun(ctx, again.ID)
	check(t, err)
	var st2 ticketimport.Stats
	check(t, json.Unmarshal(r.ImportRun.Stats, &st2))
	if r.ImportRun.Status != "done" || st2.Tickets != 2 || st2.Comments != 0 {
		t.Fatalf("second run: %s %+v", r.ImportRun.Status, st2)
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
