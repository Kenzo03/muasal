// Package testdb gives integration tests a fresh, migrated PostgreSQL database.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenzo03/muasal/server/internal/migrate"
)

// DB is a throwaway database that is dropped when the test ends.
type DB struct {
	OwnerURL string        // owner role: runs migrations
	AppURL   string        // least-privilege app role, one per database
	Pool     *pgxpool.Pool // connected as the app role
}

// New creates a database on the server named by TEST_DATABASE_URL (a role that
// may CREATE DATABASE and CREATE ROLE), migrates it and connects as its app
// role. Without TEST_DATABASE_URL the test is skipped; `make testdb` starts a
// server.
func New(t *testing.T) DB {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is not set; run `make testdb`")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to TEST_DATABASE_URL: %v", err)
	}
	name := newName()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	d := DB{OwnerURL: withDatabase(base, name)}
	// Roles are server-wide: parallel tests sharing one app role collide when
	// migrate.Up creates or alters it, so each database gets its own.
	d.AppURL = withUser(d.OwnerURL, name, "app-test-password")
	t.Cleanup(func() {
		if d.Pool != nil {
			d.Pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		_, _ = admin.Exec(context.Background(), "DROP ROLE IF EXISTS "+name)
		_ = admin.Close(context.Background())
	})
	if err := migrate.Up(ctx, d.OwnerURL, d.AppURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if d.Pool, err = pgxpool.New(ctx, d.AppURL); err != nil {
		t.Fatal(err)
	}
	return d
}

// newName names a database, and its app role too. It is random, not a
// timestamp: packages test in parallel against one server, and the clock can
// repeat (microseconds on macOS).
func newName() string {
	var b [8]byte
	rand.Read(b[:]) // never fails since Go 1.24
	return "muasal_test_" + hex.EncodeToString(b[:])
}

func withDatabase(raw, name string) string {
	u, _ := url.Parse(raw)
	u.Path = "/" + name
	return u.String()
}

func withUser(raw, user, password string) string {
	u, _ := url.Parse(raw)
	u.User = url.UserPassword(user, password)
	return u.String()
}
