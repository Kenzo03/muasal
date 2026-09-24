// Package testdb gives integration tests a fresh, migrated PostgreSQL database.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kenzo03/muasal/server/internal/migrate"
)

// DB is a throwaway database that is dropped when the test ends.
type DB struct {
	OwnerURL string        // owner role: runs migrations
	AppURL   string        // least-privilege app role
	Pool     *pgxpool.Pool // connected as the app role
}

// New creates a database on the server named by TEST_DATABASE_URL (a role that
// may CREATE DATABASE), migrates it and connects as the app role. Without
// TEST_DATABASE_URL the test is skipped; `make testdb` starts a server.
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
	name := fmt.Sprintf("muasal_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	d := DB{OwnerURL: withDatabase(base, name)}
	d.AppURL = withUser(d.OwnerURL, "app", "app-test-password")
	t.Cleanup(func() {
		if d.Pool != nil {
			d.Pool.Close()
		}
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
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
