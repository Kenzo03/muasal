package ai

import (
	"context"
	"sync"
)

// Gate limits concurrent generations to the setting ask.max_concurrent and
// keeps waiters in order, so each can be told how many questions are ahead
// (§10.3, §11.7). Embedding waits while any generation runs, so indexing never
// slows a person down.
type Gate struct {
	mu      sync.Mutex
	active  int
	queue   []*waiter
	changed chan struct{}
}

// waiter is one queued question; it has a field so every waiter has its own address.
type waiter struct{ _ byte }

// NewGate returns an idle gate.
func NewGate() *Gate { return &Gate{changed: make(chan struct{})} }

// Acquire waits for a generation slot under limit. While waiting it calls
// ahead with the number of questions in front, whenever that number changes.
// The returned release must be called once the generation ends.
func (g *Gate) Acquire(ctx context.Context, limit int, ahead func(n int)) (release func(), err error) {
	ticket := &waiter{}
	g.mu.Lock()
	g.queue = append(g.queue, ticket)
	last := -1
	for {
		pos := g.position(ticket)
		if pos == 0 && g.active < max(limit, 1) {
			g.queue = g.queue[1:]
			g.active++
			g.broadcast()
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					g.active--
					g.broadcast()
					g.mu.Unlock()
				})
			}, nil
		}
		if n := pos + g.active - max(limit, 1) + 1; n != last && ahead != nil {
			last = n
			ahead(n)
		}
		wait := g.changed
		g.mu.Unlock()
		select {
		case <-wait:
			g.mu.Lock()
		case <-ctx.Done():
			g.mu.Lock()
			g.queue = removeTicket(g.queue, ticket)
			g.broadcast()
			g.mu.Unlock()
			return nil, ctx.Err()
		}
	}
}

// WaitIdle returns once no generation runs or waits; embedders call it before
// each batch.
func (g *Gate) WaitIdle(ctx context.Context) error {
	for {
		g.mu.Lock()
		idle, wait := g.active == 0 && len(g.queue) == 0, g.changed
		g.mu.Unlock()
		if idle {
			return nil
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (g *Gate) position(t *waiter) int {
	for i, q := range g.queue {
		if q == t {
			return i
		}
	}
	return -1
}

// broadcast wakes every waiter; callers hold g.mu.
func (g *Gate) broadcast() {
	close(g.changed)
	g.changed = make(chan struct{})
}

func removeTicket(q []*waiter, t *waiter) []*waiter {
	for i, x := range q {
		if x == t {
			return append(q[:i:i], q[i+1:]...)
		}
	}
	return q
}
