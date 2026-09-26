package indexer_test

import (
	"context"
	"testing"
	"time"

	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/indexer"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// §15.4: questions older than the retention period leave the Ask log, and so
// do the threads they leave empty; newer ones stay.
func TestPurgeAskLog(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u, err := q.CreateUser(ctx, db.CreateUserParams{Email: "a@example.com", Name: "A", Locale: "id", Timezone: "Asia/Jakarta"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	thread := func(age time.Duration, questions ...time.Duration) int64 {
		var id int64
		if err := d.Pool.QueryRow(ctx, "INSERT INTO ask_threads (user_id, title, created_at) VALUES ($1, 't', $2) RETURNING id", u.ID, now.Add(-age)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, a := range questions {
			if _, err := d.Pool.Exec(ctx, `INSERT INTO ask_queries (thread_id, user_id, question, lang, scope, evidence, llm_called, status, created_at)
				VALUES ($1, $2, 'q', 'en', '{}', '[]', false, 'ai_off', $3)`, id, u.ID, now.Add(-a)); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	const day = 24 * time.Hour
	old := thread(400*day, 400*day)
	mixed := thread(400*day, 400*day, 10*day)
	fresh := thread(day, day)

	n, err := indexer.PurgeAskLog(ctx, d.Pool, 365, now)
	if err != nil || n != 2 {
		t.Fatalf("purged %d, %v; want the 2 old questions", n, err)
	}
	var left []int64
	rows, _ := d.Pool.Query(ctx, "SELECT id FROM ask_threads ORDER BY id")
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		left = append(left, id)
	}
	if len(left) != 2 || left[0] != mixed || left[1] != fresh || old == 0 {
		t.Fatalf("threads left: %v, want [%d %d]", left, mixed, fresh)
	}
	if n, err := indexer.PurgeAskLog(ctx, d.Pool, 0, now); err != nil || n != 0 {
		t.Fatalf("0 days keeps everything: %d %v", n, err)
	}
}
