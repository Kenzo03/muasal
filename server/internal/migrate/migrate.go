// Package migrate applies the embedded schema and prepares the least-privilege
// role that the app connects as (FSD §16, §19.1).
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/kenzo03/muasal/server/migrations"
)

// Up applies pending migrations as the owner role under an advisory lock, then
// River's own migrations for its job tables (FSD §2 row 3), then creates or
// updates the app role from appURL's credentials and grants it data access.
// Running it again is harmless.
func Up(ctx context.Context, ownerURL, appURL string) error {
	sqlDB, err := sql.Open("pgx", ownerURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if err := riverUp(ctx, ownerURL); err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	conn, err := pgx.Connect(ctx, ownerURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	return ensureAppRole(ctx, conn, appURL)
}

func ensureAppRole(ctx context.Context, conn *pgx.Conn, appURL string) error {
	u, err := url.Parse(appURL)
	if err != nil {
		return err
	}
	role := u.User.Username()
	password, _ := u.User.Password()
	if role == "" || password == "" {
		return errors.New("DATABASE_URL must carry the app role's user name and password")
	}
	ident := pgx.Identifier{role}.Sanitize()
	literal := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	// Check for the role first: PostgreSQL logs a failed statement with its
	// text, and this one carries the password.
	// ponytail: roles are server-wide and this runs outside the advisory lock, so
	// concurrent runs for one role fail (42710, 23505 or "tuple concurrently
	// updated") and log the password. One app replica migrates today; take an
	// advisory lock around this before running several.
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", role).Scan(&exists); err != nil {
		return fmt.Errorf("app role: %w", err)
	}
	verb := "CREATE"
	if exists {
		verb = "ALTER"
	}
	if _, err := conn.Exec(ctx, verb+" ROLE "+ident+" LOGIN PASSWORD "+literal); err != nil {
		return fmt.Errorf("app role: %w", err)
	}
	for _, stmt := range []string{
		"GRANT USAGE ON SCHEMA public TO " + ident,
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + ident,
		"GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO " + ident,
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM " + ident, // append-only (FSD §8.7)
		"REVOKE ALL ON goose_db_version FROM " + ident,
		"REVOKE ALL ON river_migration FROM " + ident,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

// riverUp creates or upgrades River's tables (river_job, river_queue, …).
func riverUp(ctx context.Context, ownerURL string) error {
	pool, err := pgxpool.New(ctx, ownerURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}
