package secret_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/kenzo03/zettra/server/internal/secret"
)

func TestSealAndOpen(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	sealed, err := secret.Seal(key, []byte("sk-test-123"))
	if err != nil || bytes.Contains(sealed, []byte("sk-test-123")) {
		t.Fatalf("seal: %x %v", sealed, err)
	}
	if got, err := secret.Open(key, sealed); err != nil || string(got) != "sk-test-123" {
		t.Fatalf("open: %q %v", got, err)
	}
	if _, err := secret.Open(bytes.Repeat([]byte{8}, 32), sealed); err == nil {
		t.Fatal("a wrong key must not open the value")
	}
	if _, err := secret.Seal(nil, []byte("x")); !errors.Is(err, secret.ErrNoKey) {
		t.Fatalf("without a key: %v", err)
	}
}
