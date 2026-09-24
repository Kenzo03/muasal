package auth

import (
	_ "embed"
	"errors"
	"strings"
	"unicode/utf8"
)

// MinPasswordLength is the FSD §15.1 minimum.
const MinPasswordLength = 12

// Policy errors; their text is the API error code the UI translates.
var (
	ErrPasswordTooShort  = errors.New("password_too_short")
	ErrPasswordTooCommon = errors.New("password_too_common")
)

// commonPasswords holds the 12+ character entries of SecLists'
// xato-net-10-million-passwords-100000.txt (MIT; see common-passwords.LICENSE).
//
//go:embed common-passwords.txt
var commonPasswords string

var common = func() map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(commonPasswords, "\n") {
		if s := strings.ToLower(strings.TrimSpace(line)); s != "" {
			m[s] = true
		}
	}
	return m
}()

// CheckPolicy returns nil for an acceptable password.
func CheckPolicy(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if common[strings.ToLower(password)] {
		return ErrPasswordTooCommon
	}
	return nil
}
