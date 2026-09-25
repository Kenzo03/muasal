package migrate_test

import (
	"context"
	"testing"

	"github.com/kenzo03/muasal/server/internal/migrate"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

func TestUpIsIdempotentAndTheAuditLogIsAppendOnly(t *testing.T) {
	d := testdb.New(t) // runs migrate.Up once
	ctx := context.Background()
	if err := migrate.Up(ctx, d.OwnerURL, d.AppURL); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO audit_events (via, entity, entity_id, action) VALUES ('system', 'test', 1, 'create')`); err != nil {
		t.Fatalf("the app role must insert audit events: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `DELETE FROM audit_events`); err == nil {
		t.Fatal("the app role must not delete audit events")
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE audit_events SET action = 'changed'`); err == nil {
		t.Fatal("the app role must not update audit events")
	}
}

// FSD §2 row 3: River's job table exists after migrate.Up, and the app role
// can queue jobs in it, but not read River's migration history.
func TestUpPreparesRiverForTheAppRole(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO river_job (args, kind, max_attempts, queue, state) VALUES ('{}', 'probe', 1, 'default', 'available')`); err != nil {
		t.Fatalf("the app role must queue River jobs: %v", err)
	}
	if _, err := d.Pool.Exec(ctx, `SELECT 1 FROM river_migration`); err == nil {
		t.Fatal("the app role must not read River's migrations")
	}
}
