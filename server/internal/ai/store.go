package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/db"
)

// Store reads the `ai` settings row, cached for five seconds, so a change in
// Admin → AI applies to the next question with no restart (§13.4).
type Store struct {
	q   *db.Queries
	ttl time.Duration
	now func() time.Time

	mu     sync.Mutex
	cached Settings
	at     time.Time
}

// NewStore reads through q.
func NewStore(q *db.Queries) *Store { return &Store{q: q, ttl: 5 * time.Second, now: time.Now} }

// Get returns the current settings, or Defaults on a fresh install.
func (st *Store) Get(ctx context.Context) (Settings, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.at.IsZero() && st.now().Sub(st.at) < st.ttl {
		return st.cached, nil
	}
	s := Defaults()
	raw, err := st.q.GetSetting(ctx, "ai")
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return Settings{}, err
	default:
		if err := json.Unmarshal(raw, &s); err != nil {
			return Settings{}, err
		}
	}
	st.cached, st.at = s, st.now()
	return s, nil
}

// Put saves s through q, which may run in the caller's transaction, and drops
// the cache so this process sees the change at once.
func (st *Store) Put(ctx context.Context, q *db.Queries, s Settings, by int64) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := q.PutSetting(ctx, db.PutSettingParams{Key: "ai", Value: raw, UpdatedBy: &by}); err != nil {
		return err
	}
	st.mu.Lock()
	st.at = time.Time{}
	st.mu.Unlock()
	return nil
}
