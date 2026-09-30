package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// SendEmails runs every minute: each user who chose email gets one message
// listing their notifications still unread after two minutes (MSL-10).
type SendEmails struct{}

func (SendEmails) Kind() string { return "send_emails" }

func (SendEmails) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 1}
}

// Worker sends the digests.
type Worker struct {
	river.WorkerDefaults[SendEmails]
	Pool      *pgxpool.Pool
	Config    Config
	PublicURL string
	Send      func(c Config, to, subject, body string) error // Send unless a test swaps it
}

func (w *Worker) Work(ctx context.Context, _ *river.Job[SendEmails]) error {
	if !w.Config.On() {
		return nil
	}
	send := w.Send
	if send == nil {
		send = Send
	}
	q := db.New(w.Pool)
	rows, err := q.PendingEmails(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for start := 0; start < len(rows); {
		end := start
		for end < len(rows) && rows[end].UserID == rows[start].UserID {
			end++
		}
		user := rows[start:end]
		items := make([]Item, len(user))
		ids := make([]int64, len(user))
		for i, r := range user {
			ids[i] = r.ID
			items[i] = Item{Type: r.Type, Actor: deref(r.ActorName), Key: deref(r.TicketKey), Title: deref(r.TicketTitle)}
			_ = json.Unmarshal(r.Payload, &items[i].Payload)
		}
		subject, body := Digest(user[0].Locale, user[0].UserName, items, w.PublicURL)
		if err := send(w.Config, user[0].Email, subject, body); err != nil {
			errs = append(errs, fmt.Errorf("email to user %d: %w", user[0].UserID, err)) // retried next minute
		} else if err := q.MarkEmailed(ctx, ids); err != nil {
			return err
		}
		start = end
	}
	return errors.Join(errs...)
}

// Item is one notification as an email line describes it.
type Item struct {
	Type, Actor, Key, Title string
	Payload                 map[string]any
}

// Digest writes one user's email: a line and a link per notification.
func Digest(locale, name string, items []Item, publicURL string) (subject, body string) {
	words := map[string]string{
		"assigned": "%s assigned you %s", "comment": "%s commented on %s", "mention": "%s mentioned you on %s",
		"status": "%s moved %s to %s", "job_done": "%s finished", "hello": "Hi %s,", "someone": "Someone",
		"many": "Muasal: %d new notifications", "footer": "Choose which notifications reach you at %s/settings/profile.",
	}
	if locale == "id" {
		words = map[string]string{
			"assigned": "%s menugaskan %s kepada Anda", "comment": "%s berkomentar di %s", "mention": "%s menyebut Anda di %s",
			"status": "%s memindahkan %s ke %s", "job_done": "%s selesai", "hello": "Halo %s,", "someone": "Seseorang",
			"many": "Muasal: %d pemberitahuan baru", "footer": "Atur pemberitahuan yang Anda terima di %s/settings/profile.",
		}
	}
	var lines []string
	var b strings.Builder
	if f := strings.Fields(name); len(f) > 0 {
		name = f[0]
	}
	fmt.Fprintf(&b, words["hello"]+"\n\n", name)
	for _, it := range items {
		actor := it.Actor
		if actor == "" {
			actor = words["someone"]
		}
		ticket := strings.TrimSpace(it.Key + " " + it.Title)
		str := func(k string) string { s, _ := it.Payload[k].(string); return s }
		var line, link string
		switch it.Type {
		case "status":
			line = fmt.Sprintf(words["status"], actor, ticket, str("status"))
		case "job_done":
			line, link = fmt.Sprintf(words["job_done"], str("name")), str("link")
		default:
			line = fmt.Sprintf(words[it.Type], actor, ticket)
		}
		if ex := str("excerpt"); ex != "" {
			line += ": “" + ex + "”"
		}
		if link == "" && it.Key != "" {
			link = "/t/" + it.Key
		}
		lines = append(lines, line)
		fmt.Fprintf(&b, "• %s\n", line)
		if link != "" {
			fmt.Fprintf(&b, "  %s%s\n", publicURL, link)
		}
	}
	fmt.Fprintf(&b, "\n"+words["footer"]+"\n", publicURL)
	subject = fmt.Sprintf(words["many"], len(items))
	if len(lines) == 1 {
		subject = "Muasal: " + lines[0]
		if r := []rune(subject); len(r) > 120 {
			subject = string(r[:119]) + "…"
		}
	}
	return subject, b.String()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Invite writes the email carrying a new user's setup link (MSL-50).
func Invite(locale, name, inviter, url string, expires time.Time) (subject, body string) {
	if f := strings.Fields(name); len(f) > 0 {
		name = f[0]
	}
	until := expires.Format("2 Jan 2006 15:04 MST")
	if locale == "id" {
		return "Undangan ke Muasal", fmt.Sprintf("Halo %s,\n\n%s mengundang Anda ke Muasal. Atur kata sandi Anda lewat tautan ini, berlaku sampai %s:\n\n  %s\n\nJika tautannya sudah tidak berlaku, minta tautan baru kepada %s.\n", name, inviter, until, url, inviter)
	}
	return "You're invited to Muasal", fmt.Sprintf("Hi %s,\n\n%s invited you to Muasal. Set your password with this link, valid until %s:\n\n  %s\n\nIf the link has expired, ask %s for a new one.\n", name, inviter, until, url, inviter)
}
