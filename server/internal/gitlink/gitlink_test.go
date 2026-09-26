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
