package httpapi_test

import (
	"net/http"
	"testing"
)

// FSD §18.2: every API response forbids sniffing, framing and outside
// referrers, and runs nothing: the API serves data, never pages.
func TestSecurityHeaders(t *testing.T) {
	e := newEnv(t)
	for _, path := range []string{"/api/v1/me", "/healthz"} {
		resp, err := http.Get(e.url + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		for k, want := range map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"Referrer-Policy":         "same-origin",
			"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
			"Cache-Control":           "no-store", // ASVS 8.2.1: nothing signed-in stays in browser caches
		} {
			if got := resp.Header.Get(k); got != want {
				t.Errorf("%s %s: %q, want %q", path, k, got, want)
			}
		}
	}
}
