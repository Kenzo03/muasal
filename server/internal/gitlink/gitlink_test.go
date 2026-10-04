package gitlink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"testing"
)

func TestVerify(t *testing.T) {
	secret, body := []byte("s3cret"), []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sum := hex.EncodeToString(mac.Sum(nil))
	cases := []struct {
		provider string
		h        http.Header
		ok       bool
	}{
		{"github", http.Header{"X-Hub-Signature-256": {"sha256=" + sum}}, true},
		{"github", http.Header{"X-Hub-Signature-256": {"sha256=00"}}, false},
		{"gitea", http.Header{"X-Gitea-Signature": {sum}}, true},
		{"gitlab", http.Header{"X-Gitlab-Token": {"s3cret"}}, true},
		{"gitlab", http.Header{"X-Gitlab-Token": {"wrong"}}, false},
		{"other", http.Header{}, false},
	}
	for _, c := range cases {
		if err := Verify(c.provider, secret, c.h, body); (err == nil) != c.ok {
			t.Errorf("%s %v: %v", c.provider, c.h, err)
		}
	}
}

func TestKeys(t *testing.T) {
	got := Keys("HRIS-12: fix overtime (see PAY-3)", "hris-4 is lowercase; HRIS-12 again", "X-1 is too short")
	if fmt.Sprint(got) != "[HRIS-12 PAY-3]" {
		t.Fatalf("%v", got)
	}
}

// MSL-29: only a key after fixes, closes or resolves counts.
func TestFixes(t *testing.T) {
	for msg, want := range map[string]string{
		"Fixes DMS-2":                         "[DMS-2]",
		"fix: DMS-2, DMS-3 and PAY-1 (see X)": "[DMS-2 DMS-3 PAY-1]",
		"Resolved HRIS-9. Refs DMS-4":         "[HRIS-9]",
		"DMS-5: fix the rounding":             "[]",
		"prefix DMS-1 and closes #12":         "[]",
	} {
		if got := fmt.Sprint(Fixes(msg)); got != want {
			t.Errorf("%q: %s, want %s", msg, got, want)
		}
	}
}

// Only http and https commit and merge-request URLs are kept.
func TestParseKeepsOnlyWebURLs(t *testing.T) {
	commits, _, _, err := Parse("github", "push", []byte(`{"commits":[{"id":"a","url":"javascript:alert(1)"},{"id":"b","url":"https://git.example.com/c/b"}]}`))
	if err != nil || len(commits) != 2 || commits[0].URL != "" || commits[1].URL != "https://git.example.com/c/b" {
		t.Fatalf("%v %+v", err, commits)
	}
	_, mr, _, err := Parse("gitlab", "merge_request", []byte(`{"object_attributes":{"iid":1,"url":"data:text/html,x"}}`))
	if err != nil || mr.URL != "" {
		t.Fatalf("%v %+v", err, mr)
	}
}
