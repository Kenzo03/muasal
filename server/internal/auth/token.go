package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// NewToken returns a 256-bit random token for cookies and links, plus its
// SHA-256 hash, which is the only form the database stores.
func NewToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b) // never fails since Go 1.24
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken maps a presented token to its stored hash.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
