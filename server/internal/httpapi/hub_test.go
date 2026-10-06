package httpapi

import "testing"

// all reaches every stream of every key without blocking on a full one; the
// listener uses it to ask open pages to resync after it reconnects.
func TestHubAllReachesEverySubscriber(t *testing.T) {
	var h hub
	a, stopA := h.subscribe(1)
	defer stopA()
	b, stopB := h.subscribe(2)
	defer stopB()
	full, stopFull := h.subscribe(3)
	defer stopFull()
	for range cap(full) {
		full <- 9
	}
	h.all(0) // must not block on the full channel
	for name, ch := range map[string]chan int64{"a": a, "b": b} {
		select {
		case id := <-ch:
			if id != 0 {
				t.Fatalf("%s got %d", name, id)
			}
		default:
			t.Fatalf("%s got nothing", name)
		}
	}
}
