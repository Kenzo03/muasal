package mail

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

func TestDigest(t *testing.T) {
	subject, body := Digest("id", "Bayu Santoso", []Item{
		{Type: "assigned", Actor: "Rina Kusuma", Key: "DMS-1", Title: "Turunkan batas persetujuan"},
		{Type: "mention", Actor: "Rina Kusuma", Key: "DMS-1", Title: "Turunkan batas persetujuan", Payload: map[string]any{"excerpt": "@bayu tolong cek"}},
		{Type: "status", Actor: "Rina Kusuma", Key: "DMS-6", Title: "Batas kredit", Payload: map[string]any{"status": "In progress"}},
	}, "https://muasal.example.com")
	for _, want := range []string{"Halo Bayu,", "• Rina Kusuma menugaskan DMS-1 Turunkan batas persetujuan kepada Anda", "  https://muasal.example.com/t/DMS-1",
		"menyebut Anda di DMS-1 Turunkan batas persetujuan: “@bayu tolong cek”", "memindahkan DMS-6 Batas kredit ke In progress", "/settings/profile"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if subject != "Muasal: 3 pemberitahuan baru" {
		t.Errorf("subject %q", subject)
	}
	if subject, _ := Digest("en", "Dewi", []Item{{Type: "job_done", Payload: map[string]any{"name": "jira.csv", "link": "/admin/imports/1"}}}, "http://x"); subject != "Muasal: jira.csv finished" {
		t.Errorf("one item: %q", subject)
	}
}

// Send talks SMTP; a relay without TLS takes the message as written.
func TestSend(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	got := make(chan string, 1)
	serve := func(c net.Conn) {
		defer c.Close()
		r, w := bufio.NewReader(c), bufio.NewWriter(c)
		say := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }
		say("220 fake")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			switch {
			case inData && line == ".\r\n":
				inData = false
				say("250 queued")
				got <- data.String()
			case inData:
				data.WriteString(line)
			case strings.HasPrefix(line, "EHLO"):
				say("250 fake")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				say("354 go on")
			case strings.HasPrefix(line, "QUIT"):
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(c)
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	c := Config{Host: "127.0.0.1", Port: port, From: "muasal@example.com", TLS: "none"}
	if err := Send(c, "bayu@example.com", "Muasal: 1 pemberitahuan", "Halo Bayu,\n\n• baris"); err != nil {
		t.Fatal(err)
	}
	msg := <-got
	for _, want := range []string{"From: muasal@example.com\r\n", "To: bayu@example.com\r\n", "Content-Type: text/plain; charset=utf-8\r\n", "Halo Bayu,\r\n", "• baris\r\n"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
	if err := Send(Config{Host: "127.0.0.1", Port: port, From: "m@example.com", TLS: "starttls"}, "b@example.com", "s", "b"); err == nil {
		t.Error("STARTTLS was not required") // the fake offers none
	}
}

// MSL-10: one message per user who chose email, for notifications still unread
// two minutes on; each goes out once.
func TestWorkerSendsOneDigestPerUser(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	on, err := q.CreateUser(ctx, db.CreateUserParams{Email: "bayu@example.com", Name: "Bayu Santoso", Locale: "id", Timezone: "Asia/Jakarta"})
	if err != nil {
		t.Fatal(err)
	}
	off, err := q.CreateUser(ctx, db.CreateUserParams{Email: "dewi@example.com", Name: "Dewi", Locale: "id", Timezone: "Asia/Jakarta"})
	if err != nil {
		t.Fatal(err)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE users SET notify_prefs = '{"email": true}' WHERE id = $1`, on.ID)
	for _, n := range []struct {
		user        int64
		name, since string
		read        bool
	}{
		{on.ID, "a.csv", "5 minutes", false}, {on.ID, "b.csv", "4 minutes", false}, {on.ID, "read.csv", "5 minutes", true},
		{on.ID, "fresh.csv", "10 seconds", false}, {off.ID, "c.csv", "5 minutes", false},
	} {
		exec(`INSERT INTO notifications (user_id, type, payload, created_at, read_at)
			VALUES ($1, 'job_done', jsonb_build_object('name', $2::text, 'link', '/admin/imports/1'), now() - $3::interval, CASE WHEN $4 THEN now() END)`,
			n.user, n.name, n.since, n.read)
	}
	var sent []string
	w := &Worker{Pool: d.Pool, Config: Config{Host: "smtp.example.com", From: "muasal@example.com"}, PublicURL: "https://m.example.com",
		Send: func(_ Config, to, subject, body string) error {
			sent = append(sent, to+"|"+subject+"|"+body)
			return nil
		}}
	for range 2 {
		if err := w.Work(ctx, &river.Job[SendEmails]{}); err != nil {
			t.Fatal(err)
		}
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "bayu@example.com|Muasal: 2 pemberitahuan baru|") ||
		!strings.Contains(sent[0], "a.csv selesai") || !strings.Contains(sent[0], "b.csv selesai") || strings.Contains(sent[0], "fresh.csv") {
		t.Fatalf("sent: %q", sent)
	}
}

// MSL-52: a due reminder reads as one.
func TestDigestDue(t *testing.T) {
	subject, body := Digest("id", "Fajar Nugroho", []Item{{Type: "due", Key: "HRIS-8", Title: "Cut-off payroll", Payload: map[string]any{"when": "tomorrow"}}}, "https://m.example")
	if subject != "Muasal: HRIS-8 Cut-off payroll jatuh tempo besok" || !strings.Contains(body, "https://m.example/t/HRIS-8") {
		t.Fatalf("%q\n%s", subject, body)
	}
}
