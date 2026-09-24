package auth

import (
	"bytes"
	"testing"
)

func TestNewToken(t *testing.T) {
	a, hashA := NewToken()
	b, _ := NewToken()
	if a == b || len(a) != 43 {
		t.Fatalf("tokens %q and %q", a, b)
	}
	if !bytes.Equal(hashA, HashToken(a)) || len(hashA) != 32 {
		t.Fatal("the hash must be the SHA-256 of the token")
	}
}
