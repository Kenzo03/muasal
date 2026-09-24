package auth

import (
	"errors"
	"testing"
)

func TestCheckPolicy(t *testing.T) {
	cases := []struct {
		password string
		want     error
	}{
		{"short", ErrPasswordTooShort},
		{"elevenchars", ErrPasswordTooShort},   // 11 characters
		{"1qaz2wsx3edc", ErrPasswordTooCommon}, // in the SecLists top 100,000
		{"1QAZ2WSX3EDC", ErrPasswordTooCommon}, // changing case does not help
		{"kopi-susu-di-kantor-7", nil},
	}
	for _, c := range cases {
		if got := CheckPolicy(c.password); !errors.Is(got, c.want) {
			t.Errorf("CheckPolicy(%q) = %v, want %v", c.password, got, c.want)
		}
	}
	if len(common) < 400 {
		t.Fatalf("the common list looks truncated: %d entries", len(common))
	}
}
