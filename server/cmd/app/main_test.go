package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kenzo03/zettra/server/internal/config"
)

func TestRunWithoutACommandPrintsUsage(t *testing.T) {
	err := run(context.Background(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("got %v", err)
	}
}

func TestHealthcheck(t *testing.T) {
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ready.Close()
	if err := healthcheck(config.Config{ListenAddr: strings.TrimPrefix(ready.URL, "http://")}); err != nil {
		t.Fatal(err)
	}
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	if err := healthcheck(config.Config{ListenAddr: strings.TrimPrefix(down.URL, "http://")}); err == nil {
		t.Fatal("want an error when the API is not ready")
	}
}
