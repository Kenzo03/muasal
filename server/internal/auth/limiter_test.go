package auth

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	clock := time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)
	l := NewLimiter(20, time.Minute)
	l.now = func() time.Time { return clock }
	for i := 1; i <= 20; i++ {
		if !l.Allow("10.0.0.1") {
			t.Fatalf("attempt %d blocked", i)
		}
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("the 21st attempt in one minute was allowed")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("other addresses must not be affected")
	}
	clock = clock.Add(time.Minute)
	if !l.Allow("10.0.0.1") {
		t.Fatal("a new window must reset the count")
	}
}
