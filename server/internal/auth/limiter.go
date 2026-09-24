package auth

import (
	"sync"
	"time"
)

// Limiter allows at most limit events per key within each fixed window.
// ponytail: in memory and per process; move it to PostgreSQL if the API ever runs as several processes.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	start  time.Time
	counts map[string]int
	now    func() time.Time
}

// NewLimiter returns a limiter on the wall clock.
func NewLimiter(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, counts: map[string]int{}, now: time.Now}
}

// Allow records one event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := l.now(); now.Sub(l.start) >= l.window {
		l.start, l.counts = now, map[string]int{}
	}
	l.counts[key]++
	return l.counts[key] <= l.limit
}
