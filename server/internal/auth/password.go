// Package auth holds the sign-in building blocks: password hashing and policy,
// random tokens and the per-IP rate limiter (FSD §15.1, §18.2).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id at the OWASP minimum: 19 MiB of memory, 2 passes, 1 lane.
const (
	argonMemory  = 19 * 1024
	argonPasses  = 2
	argonThreads = 1
	argonKeyLen  = 32
)

// HashPassword returns an argon2id hash in PHC string format.
func HashPassword(password string) string {
	salt := make([]byte, 16)
	rand.Read(salt) // never fails since Go 1.24
	key := argon2.IDKey([]byte(password), salt, argonPasses, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonPasses, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// CheckPassword reports whether password matches hash. The parameters come
// from the hash itself, so raising the constants later keeps old hashes valid.
func CheckPassword(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var version int
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil {
		return false
	}
	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
