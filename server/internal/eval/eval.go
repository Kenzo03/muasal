// Package eval runs the Ask quality gate (FSD §11.8): a golden set of
// questions against the current AI settings, reporting citation precision,
// evidence recall@12, abstention and median latency. Golden set v0 runs on a
// demo HRIS dataset that `app eval --seed` loads into project DEMO.
package eval

import (
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenzo03/zettra/server/internal/ai"
	"github.com/kenzo03/zettra/server/internal/ask"
	"github.com/kenzo03/zettra/server/internal/db"
)

//go:embed data/demo-hris.json data/golden-v0.jsonl
var files embed.FS

// Question is one line of a golden set. Expected lists the keys a good answer
// cites; empty means the question is unanswerable and Ask must abstain.
type Question struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Expected []string `json:"expected"`
	Node     string   `json:"node,omitempty"`    // an explicit node chip, by path: "HR › Attendance › Overtime Approval"
	Client   string   `json:"client,omitempty"`  // an explicit client chip, by name
	Follows  string   `json:"follows,omitempty"` // a follow-up in the thread of this earlier question (§11.9)
}

// LoadQuestions reads a JSONL golden set; "" reads the embedded v0.
func LoadQuestions(path string) ([]Question, error) {
	raw, err := read(path, "data/golden-v0.jsonl")
	if err != nil {
		return nil, err
	}
	var out []Question
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var q Question
		if err := json.Unmarshal([]byte(line), &q); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		out = append(out, q)
	}
	return out, nil
}

func read(path, embedded string) ([]byte, error) {
	if path == "" {
		return files.ReadFile(embedded)
	}
	return os.ReadFile(path)
}

// Targets of §11.8. The latency target is reported, but the dev laptop is
// expected to miss it (§18.1), so only Strict makes it fail the run.
type Targets struct {
	Precision, Recall, Abstention float64
	Latency                       time.Duration
	Strict                        bool
}

// DefaultTargets are §11.8's.
var DefaultTargets = Targets{Precision: 0.90, Recall: 0.85, Abstention: 1.0, Latency: 15 * time.Second}

// Row is one question's outcome.
type Row struct {
	Question Question
	Status   string
	Cited    []string
	Evidence []string // the first 12 evidence keys
	Latency  time.Duration
}

// Report is the run's four measures.
type Report struct {
	Rows                          []Row
	Precision, Recall, Abstention float64
	MedianLatency                 time.Duration
	Model                         string
}

// Pass reports whether the report meets the targets.
func (r Report) Pass(t Targets) bool {
	ok := r.Precision >= t.Precision && r.Recall >= t.Recall && r.Abstention >= t.Abstention
	if t.Strict {
		ok = ok && r.MedianLatency <= t.Latency
	}
	return ok
}

// Run asks every question as asker, in the DEMO project unless a question's
// chips say otherwise, with a fixed seed (§11.5).
func Run(ctx context.Context, pool *pgxpool.Pool, rt *ai.Runtime, asker ask.Asker, questions []Question, seed int, progress io.Writer) (Report, error) {
	s, err := rt.Store.Get(ctx)
	if err != nil {
		return Report{}, err
	}
	if s.Mode == ai.ModeOff {
		return Report{}, errors.New("AI is turned off; set Admin → AI to Local or Bring your own key first")
	}
	q := db.New(pool)
	engine := ask.NewEngine(pool, rt)
	engine.Seed = &seed
	rep := Report{Model: s.Badge()}
	threads := map[string]int64{}
	for i, gq := range questions {
		explicit, err := chips(ctx, q, gq)
		if err != nil {
			return rep, fmt.Errorf("%s: %w", gq.ID, err)
		}
		var evidence []string
		start := time.Now()
		req := ask.Request{Asker: asker, Question: gq.Question, Explicit: explicit}
		if gq.Follows != "" {
			th, ok := threads[gq.Follows]
			if !ok {
				return rep, fmt.Errorf("%s follows %s, which has not run", gq.ID, gq.Follows)
			}
			req.ThreadID = &th
		}
		res, err := engine.Ask(ctx, req, ask.Sink{
			Evidence: func(items []ask.Item) {
				for _, it := range items[:min(12, len(items))] {
					evidence = append(evidence, it.Key)
				}
			},
		})
		if err != nil {
			return rep, fmt.Errorf("%s: %w", gq.ID, err)
		}
		threads[gq.ID] = res.ThreadID
		row := Row{Question: gq, Status: res.Status, Evidence: evidence, Latency: time.Since(start)}
		for _, c := range res.Claims {
			for _, k := range c.Cites {
				if !slices.Contains(row.Cited, k) {
					row.Cited = append(row.Cited, k)
				}
			}
		}
		rep.Rows = append(rep.Rows, row)
		if progress != nil {
			fmt.Fprintf(progress, "%d/%d %s %s %s\n", i+1, len(questions), gq.ID, row.Status, row.Latency.Round(100*time.Millisecond))
		}
	}
	rep.score()
	return rep, nil
}

// score computes the measures of §11.8 over the rows.
func (r *Report) score() {
	var cited, citedOK, expected, found, unanswerable, abstained int
	var latencies []time.Duration
	for _, row := range r.Rows {
		latencies = append(latencies, row.Latency)
		if len(row.Question.Expected) == 0 {
			unanswerable++
			if row.Status == ask.StatusNotEnough {
				abstained++
			}
			continue
		}
		for _, k := range row.Cited {
			cited++
			if slices.Contains(row.Question.Expected, k) {
				citedOK++
			}
		}
		for _, k := range row.Question.Expected {
			expected++
			if slices.Contains(row.Evidence, k) {
				found++
			}
		}
	}
	r.Precision, r.Recall, r.Abstention = ratio(citedOK, cited), ratio(found, expected), ratio(abstained, unanswerable)
	if len(latencies) > 0 {
		slices.Sort(latencies)
		r.MedianLatency = latencies[len(latencies)/2]
	}
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

// chips turns a question's node path and client name into an explicit scope.
func chips(ctx context.Context, q *db.Queries, gq Question) (ask.Scope, error) {
	var s ask.Scope
	if gq.Node != "" {
		id, err := q.GetNodeIDByPath(ctx, db.GetNodeIDByPathParams{ProjectKey: DemoKey, Path: splitPath(gq.Node)})
		if err != nil {
			return s, fmt.Errorf("node %q: %w", gq.Node, err)
		}
		s.NodeIDs = []int64{id}
	}
	if gq.Client != "" {
		c, err := q.GetClientByName(ctx, gq.Client)
		if err != nil {
			return s, fmt.Errorf("client %q: %w", gq.Client, err)
		}
		s.ClientIDs = []int64{c.ID}
	}
	return s, nil
}

func splitPath(p string) []string {
	parts := strings.Split(p, "›")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// Print writes the per-question table and the four measures against targets.
func (r Report) Print(w io.Writer, t Targets) {
	fmt.Fprintf(w, "\nModel: %s\n\n%-5s %-16s %-9s %-26s %s\n", r.Model, "ID", "STATUS", "LATENCY", "CITED", "EXPECTED")
	for _, row := range r.Rows {
		fmt.Fprintf(w, "%-5s %-16s %-9s %-26s %s\n", row.Question.ID, row.Status, row.Latency.Round(100*time.Millisecond),
			cmp.Or(strings.Join(row.Cited, " "), "-"), cmp.Or(strings.Join(row.Question.Expected, " "), "(unanswerable)"))
	}
	mark := func(ok bool) string {
		if ok {
			return "pass"
		}
		return "MISS"
	}
	fmt.Fprintf(w, "\nCitation precision  %5.1f%%  target %3.0f%%  %s\n", 100*r.Precision, 100*t.Precision, mark(r.Precision >= t.Precision))
	fmt.Fprintf(w, "Evidence recall@12  %5.1f%%  target %3.0f%%  %s\n", 100*r.Recall, 100*t.Recall, mark(r.Recall >= t.Recall))
	fmt.Fprintf(w, "Abstention          %5.1f%%  target %3.0f%%  %s\n", 100*r.Abstention, 100*t.Abstention, mark(r.Abstention >= t.Abstention))
	latency := mark(r.MedianLatency <= t.Latency)
	if !t.Strict && r.MedianLatency > t.Latency {
		latency = "missed (reported only; --strict-latency fails the run)"
	}
	fmt.Fprintf(w, "Median latency      %6s  target %s  %s\n", r.MedianLatency.Round(100*time.Millisecond), t.Latency, latency)
}
