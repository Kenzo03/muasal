package ai_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/ai"
)

// §11.7: with one slot, a second question waits, is told it has one question
// ahead, and starts when the first ends; embedding waits for both.
func TestGateQueuesInOrderAndEmbeddingWaits(t *testing.T) {
	g := ai.NewGate()
	ctx := context.Background()
	release1, err := g.Acquire(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var told []int
	started := make(chan func())
	go func() {
		release2, _ := g.Acquire(ctx, 1, func(n int) { mu.Lock(); told = append(told, n); mu.Unlock() })
		started <- release2
	}()
	idle := make(chan struct{})
	go func() { _ = g.WaitIdle(ctx); close(idle) }()

	select {
	case <-started:
		t.Fatal("the second question started while the slot was taken")
	case <-time.After(50 * time.Millisecond):
	}
	release1()
	release2 := <-started
	mu.Lock()
	if len(told) != 1 || told[0] != 1 {
		t.Fatalf("questions ahead: %v", told)
	}
	mu.Unlock()
	select {
	case <-idle:
		t.Fatal("embedding resumed during a generation")
	case <-time.After(20 * time.Millisecond):
	}
	release2()
	release2() // a second call is harmless
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("embedding did not resume")
	}
}

// A waiter that gives up leaves the queue, so the next one is not stuck behind it.
func TestGateDropsAbandonedWaiters(t *testing.T) {
	g := ai.NewGate()
	release, _ := g.Acquire(context.Background(), 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.Acquire(ctx, 1, nil); err == nil {
		t.Fatal("the waiter should time out")
	}
	release()
	if err := g.WaitIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r, err := g.Acquire(context.Background(), 2, nil); err != nil {
		t.Fatal(err)
	} else {
		r()
	}
}
