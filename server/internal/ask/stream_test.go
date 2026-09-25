package ask_test

import (
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ask"
)

// §11.5: each claim is emitted as soon as its object closes, however the
// stream is split, and before the answer ends.
func TestClaimsStreamOneByOne(t *testing.T) {
	answer := `{"claims":[{"text":"Overtime skips the supervisor for Client A.","cites":["HRIS-231"]},{"text":"Rina confirmed it by phone.","cites":["HRIS-231","HRIS-240"]}]}`
	pr, pw := io.Pipe()
	got := make(chan ask.Claim, 2)
	done := make(chan error)
	go func() { done <- ask.Claims(pr, func(c ask.Claim) { got <- c }) }()
	cut := strings.Index(answer, `},{`) + 1
	for i := 0; i < cut; i += 5 { // the first claim, in pieces
		pw.Write([]byte(answer[i:min(i+5, cut)]))
	}
	select {
	case c := <-got:
		if c.Text != "Overtime skips the supervisor for Client A." {
			t.Fatalf("first claim: %+v", c)
		}
	case <-time.After(time.Second):
		t.Fatal("the first claim waited for the whole answer")
	}
	pw.Write([]byte(answer[cut:]))
	pw.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if c := <-got; !slices.Equal(c.Cites, []string{"HRIS-231", "HRIS-240"}) {
		t.Fatalf("second claim: %+v", c)
	}
}

// Invalid JSON is an error, after the claims that came before it; a wrapped
// JSON-mode answer still parses.
func TestClaimsHandlesBrokenAndWrappedAnswers(t *testing.T) {
	var n int
	err := ask.Claims(strings.NewReader(`{"claims":[{"text":"One.","cites":["A-1"]},{"text":"Two`), func(ask.Claim) { n++ })
	if err == nil || n != 1 {
		t.Fatalf("broken: %v, %d claims", err, n)
	}
	n = 0
	if err := ask.Claims(strings.NewReader("```json\n{\"note\":\"x\",\"claims\":[{\"text\":\"One.\",\"cites\":[\"A-1\"]}]}\n```"), func(ask.Claim) { n++ }); err != nil || n != 1 {
		t.Fatalf("wrapped: %v, %d claims", err, n)
	}
}

// AC-AK-6: citations outside the evidence go; a claim left without one goes;
// a claim naming an outside key goes.
func TestValidateDropsWhatTheEvidenceDoesNotBack(t *testing.T) {
	evidence := []string{"HRIS-231", "HRIS-240"}
	for _, c := range []struct {
		in     ask.Claim
		cites  []string
		reason string
	}{
		{ask.Claim{Text: "Skips the supervisor.", Cites: []string{"HRIS-231"}}, []string{"HRIS-231"}, ""},
		{ask.Claim{Text: "Skips the supervisor.", Cites: []string{"hris-231", "HRIS-999", "HRIS-231"}}, []string{"HRIS-231"}, "citation_removed"},
		{ask.Claim{Text: "Skips the supervisor.", Cites: []string{"HRIS-999"}}, nil, "no_citation"},
		{ask.Claim{Text: "Like HRIS-999, it skips the supervisor.", Cites: []string{"HRIS-231"}}, nil, "outside_key"},
		{ask.Claim{Text: "  ", Cites: []string{"HRIS-231"}}, nil, "empty"},
	} {
		got, dropped := ask.Validate(c.in, evidence)
		reason := ""
		if dropped != nil {
			reason = dropped.Reason
		}
		if !slices.Equal(got.Cites, c.cites) || reason != c.reason {
			t.Errorf("%+v: %+v, dropped %q", c.in, got, reason)
		}
	}
}

// §11.5: the citation enum is exactly the evidence keys.
func TestSchemaListsTheEvidenceKeys(t *testing.T) {
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
	if err := json.Unmarshal(ask.Schema([]string{"HRIS-231", "HRIS-240"}), &s); err != nil {
		t.Fatal(err)
	}
	if got := s.Properties.Claims.Items.Properties.Cites.Items.Enum; !slices.Equal(got, []string{"HRIS-231", "HRIS-240"}) {
		t.Fatalf("enum: %v", got)
	}
	if !strings.Contains(ask.System("id"), "Answer in Bahasa Indonesia") || !strings.Contains(ask.User("Why?", "[HRIS-231] …"), "EVIDENCE:\n<<<\n[HRIS-231] …\n>>>") {
		t.Fatal("prompt")
	}
}
