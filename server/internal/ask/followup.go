package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Follow-up questions (FSD §11.9) reuse the thread instead of asking the model
// to rewrite the question, so they add no model call.

// conversationChars is the CONVERSATION block's budget: 600 tokens at 3.5
// characters a token (§11.4), counted inside the context budget.
const conversationChars = 2100

// turn is an earlier question in the thread with its validated claims.
type turn struct {
	question string
	claims   []Claim
	explicit Scope
	detected Detected
}

// history reads the thread's last two turns, oldest first; none without a thread.
func (e *Engine) history(ctx context.Context, threadID *int64) ([]turn, error) {
	if threadID == nil {
		return nil, nil
	}
	rows, err := e.q.ListThreadQueries(ctx, threadID)
	if err != nil {
		return nil, err
	}
	var out []turn
	for _, r := range rows[max(0, len(rows)-2):] {
		t := turn{question: r.Question}
		var scope struct {
			Explicit Scope    `json:"explicit"`
			Detected Detected `json:"detected"`
		}
		if err := json.Unmarshal(r.Answer, &t.claims); r.Answer != nil && err != nil {
			return nil, err
		}
		if err := json.Unmarshal(r.Scope, &scope); err != nil {
			return nil, err
		}
		t.explicit, t.detected = scope.Explicit, scope.Detected
		out = append(out, t)
	}
	return out, nil
}

// CarryOver keeps the previous question's detected chips of every kind the new
// question does not name, so "And for Client B?" swaps Client A for Client B
// and keeps the menu (§11.9). Users and contacts are one kind, people; so are
// the two ends of a date range.
func CarryOver(prev, now Detected) Detected {
	out := now
	keep := map[string]bool{}
	if len(now.ClientIDs) == 0 && !now.AllClients && len(prev.ClientIDs) > 0 {
		out.ClientIDs, keep["client"] = prev.ClientIDs, true
	}
	if len(now.NodeIDs) == 0 && len(prev.NodeIDs) > 0 {
		out.NodeIDs, keep["node"] = prev.NodeIDs, true
	}
	if len(now.UserIDs) == 0 && len(now.ContactIDs) == 0 && len(prev.UserIDs)+len(prev.ContactIDs) > 0 {
		out.UserIDs, out.ContactIDs, keep["user"], keep["contact"] = prev.UserIDs, prev.ContactIDs, true, true
	}
	if now.From == nil && now.To == nil {
		out.From, out.To = prev.From, prev.To
	}
	out.Labels = slices.Clone(now.Labels)
	for _, l := range prev.Labels {
		if keep[l.Kind] {
			out.Labels = append(out.Labels, l)
		}
	}
	return out
}

// followUp is what the thread adds to a new question.
type followUp struct {
	retrieval    string   // the question plus the previous one
	cited        []string // keys the previous answer cited, when the client scope is unchanged
	conversation string   // the CONVERSATION block; "" without history
}

// plan builds the follow-up from the thread's turns for a question whose
// scope is final.
func plan(turns []turn, question string, scope Scope) followUp {
	f := followUp{retrieval: question}
	if len(turns) == 0 {
		return f
	}
	last := turns[len(turns)-1]
	f.retrieval = question + "\n" + last.question
	// The previous evidence builds the answer only while the question stays
	// on the same clients: "And for Client B?" must not cite Client A (AC-AK-9).
	if slices.Equal(Merge(last.explicit, last.detected).ClientIDs, scope.ClientIDs) {
		for _, c := range last.claims {
			for _, k := range c.Cites {
				if !slices.Contains(f.cited, k) {
					f.cited = append(f.cited, k)
				}
			}
		}
	}
	// Newest turns first into the budget, then written oldest first.
	var blocks []string
	used := 0
	for i := len(turns) - 1; i >= 0; i-- {
		var b strings.Builder
		fmt.Fprintf(&b, "Q: %s\n", turns[i].question)
		if len(turns[i].claims) == 0 {
			b.WriteString("A: (not enough information)\n")
		}
		for _, c := range turns[i].claims {
			fmt.Fprintf(&b, "A: %s [%s]\n", c.Text, strings.Join(c.Cites, ", "))
		}
		text := strings.TrimSpace(b.String())
		if n := len([]rune(text)); used+n > conversationChars {
			if used == 0 {
				blocks = append(blocks, cut(text, conversationChars))
			}
			break
		} else {
			used += n
		}
		blocks = append([]string{text}, blocks...)
	}
	f.conversation = strings.Join(blocks, "\n\n")
	return f
}
