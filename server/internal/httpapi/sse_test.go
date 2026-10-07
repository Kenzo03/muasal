package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A server-sent event stream must tell proxies not to transform it: the
// Next.js dev server (and any gzipping proxy) otherwise compresses it and
// holds small events in the gzip buffer, so live updates never arrive.
func TestSSEForbidsTransforms(t *testing.T) {
	w := httptest.NewRecorder()
	_, stop := startSSE(w)
	stop()
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-transform") {
		t.Fatalf("Cache-Control %q: a gzipping proxy would buffer the events", cc)
	}
}
