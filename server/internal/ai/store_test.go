package ai_test

import (
	"context"
	"testing"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/testdb"
)

// §13.4: a saved change applies at once in this process; a fresh install reads Defaults.
func TestStoreReadsDefaultsThenSavedSettings(t *testing.T) {
	d := testdb.New(t)
	ctx := context.Background()
	q := db.New(d.Pool)
	u, err := q.CreateUser(ctx, db.CreateUserParams{Email: "a@example.com", Name: "A", Locale: "id", Timezone: "Asia/Jakarta", IsAdmin: true})
	if err != nil {
		t.Fatal(err)
	}
	st := ai.NewStore(q)
	s, err := st.Get(ctx)
	if err != nil || s.Mode != ai.ModeOff {
		t.Fatalf("fresh install: %+v %v", s, err)
	}
	s.Mode = ai.ModeLocal
	s.Chat.URL = "http://host.docker.internal:11434/v1"
	if err := st.Put(ctx, q, s, u.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Get(ctx); err != nil || got.Mode != ai.ModeLocal || got.Chat.URL != s.Chat.URL {
		t.Fatalf("after Put: %+v %v", got, err)
	}
}
