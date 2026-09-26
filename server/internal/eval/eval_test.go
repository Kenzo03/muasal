package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/llm/llmtest"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// The embedded golden set follows §11.8: 10 unanswerable questions, at least
// 40% Indonesian or mixed, and every expected key exists in the dataset; and
// §11.9: 10 follow-ups, each after the question it follows.
func TestGoldenSetV0IsWellFormed(t *testing.T) {
	qs, err := LoadQuestions("")
	if err != nil {
		t.Fatal(err)
	}
	ds, err := LoadDataset("")
	if err != nil {
		t.Fatal(err)
	}
	unanswerable, indonesian, followUps := 0, 0, 0
	seen := map[string]bool{}
	for _, q := range qs {
		if q.Follows != "" {
			followUps++
			if !seen[q.Follows] {
				t.Errorf("%s follows %s, which comes later or not at all", q.ID, q.Follows)
			}
		}
		seen[q.ID] = true
		if len(q.Expected) == 0 {
			unanswerable++
		}
		if ask.Language(q.Question, "en") == "id" {
			indonesian++
		}
		for _, k := range q.Expected {
			var n int
			if _, err := fmt.Sscanf(k, "DEMO-%d", &n); err != nil || n < 1 || n > len(ds.Tickets) {
				t.Errorf("%s expects %s, which the dataset lacks", q.ID, k)
			}
		}
	}
	if unanswerable != 10 || followUps != 10 || float64(indonesian)/float64(len(qs)) < 0.4 {
		t.Fatalf("%d questions: %d unanswerable, %d follow-ups, %d Indonesian", len(qs), unanswerable, followUps, indonesian)
	}
}

// §11.8's measures: precision over cited keys, recall over expected keys in
// the first 12 evidence keys, abstention over unanswerable questions.
func TestScore(t *testing.T) {
	r := Report{Rows: []Row{
		{Question: Question{Expected: []string{"D-1"}}, Cited: []string{"D-1", "D-2"}, Evidence: []string{"D-1"}, Latency: 3 * time.Second},
		{Question: Question{Expected: []string{"D-3", "D-4"}}, Cited: []string{"D-3"}, Evidence: []string{"D-3"}, Latency: 9 * time.Second},
		{Question: Question{}, Status: ask.StatusNotEnough, Latency: time.Second},
		{Question: Question{}, Status: ask.StatusAnswered, Latency: 20 * time.Second},
	}}
	r.score()
	if r.Precision != 2.0/3 || r.Recall != 2.0/3 || r.Abstention != 0.5 || r.MedianLatency != 9*time.Second {
		t.Fatalf("scores: %+v", r)
	}
	if r.Pass(DefaultTargets) {
		t.Fatal("these scores must not pass")
	}
}

// app eval end to end on the demo dataset, against a fake model server that
// cites the first evidence key: the dataset loads once, and every question runs.
func TestSeedAndRun(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	fake := llmtest.New(t)
	fake.Answer = func(_, _ string, schema json.RawMessage) string {
		var s struct {
			Properties struct {
				Claims struct {
					Items struct {
						Properties struct {
							Cites struct {
								Items struct {
									Enum []string `json:"enum"`
								} `json:"items"`
							} `json:"cites"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"claims"`
			} `json:"properties"`
		}
		_ = json.Unmarshal(schema, &s)
		return fmt.Sprintf(`{"claims":[{"text":"The evidence answers it.","cites":[%q]}]}`, s.Properties.Claims.Items.Properties.Cites.Items.Enum[0])
	}
	admin, err := q.CreateUser(ctx, db.CreateUserParams{Email: "eval@example.com", Name: "Eval", Locale: "en", Timezone: "Asia/Jakarta", IsAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	rt := &ai.Runtime{Store: ai.NewStore(q), Gate: ai.NewGate()}
	s := ai.Defaults()
	s.Mode, s.Chat.URL, s.Embed.URL = ai.ModeLocal, fake.BaseURL(), fake.BaseURL()
	if err := rt.Store.Put(ctx, q, s, admin.ID); err != nil {
		t.Fatal(err)
	}
	ds, _ := LoadDataset("")
	for range 2 { // a second seed leaves the project as it is
		if err := Seed(ctx, d.Pool, rt, ds, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	var tickets, chunks int
	_ = d.Pool.QueryRow(ctx, "SELECT count(*) FROM tickets").Scan(&tickets)
	_ = d.Pool.QueryRow(ctx, "SELECT count(*) FROM chunks WHERE embedding IS NOT NULL").Scan(&chunks)
	if tickets != 48 || chunks < 48 {
		t.Fatalf("seeded %d tickets, %d embedded chunks", tickets, chunks)
	}
	qs, _ := LoadQuestions("")
	asker := ask.Asker{UserID: admin.ID, IsAdmin: true, Locale: "en", TZ: time.UTC}
	rep, err := Run(ctx, d.Pool, rt, asker, qs, 7, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	rep.Print(&out, DefaultTargets)
	if len(rep.Rows) != len(qs) || rep.Recall == 0 || !strings.Contains(out.String(), "Citation precision") {
		t.Fatalf("report:\n%s", out.String())
	}
	t.Logf("with the fake server:%s", out.String()[strings.Index(out.String(), "\nCitation"):])
}
