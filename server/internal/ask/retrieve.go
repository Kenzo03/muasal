package ask

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/kenzo03/muasal/server/internal/db"
)

// Scope is the final set of chips a question runs under: explicit ones, then
// detected ones where no explicit chip of the kind exists (§10.2, §11.2).
// Empty means no filter. From and To are days in the asker's timezone; To is
// inclusive.
type Scope struct {
	ProjectIDs []int64    `json:"project_ids,omitempty"`
	NodeIDs    []int64    `json:"node_ids,omitempty"`
	ClientIDs  []int64    `json:"client_ids,omitempty"`
	UserIDs    []int64    `json:"user_ids,omitempty"`
	ContactIDs []int64    `json:"contact_ids,omitempty"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
}

// Merge adds the detected chips of each kind the explicit scope leaves open.
// Detected chips only narrow; where they conflict with an explicit chip, the
// explicit one wins (§11.2).
func Merge(explicit Scope, d Detected) Scope {
	s := explicit
	if len(s.NodeIDs) == 0 {
		s.NodeIDs = d.NodeIDs
	}
	if len(s.ClientIDs) == 0 {
		s.ClientIDs = d.ClientIDs
	}
	if len(s.UserIDs) == 0 && len(s.ContactIDs) == 0 {
		s.UserIDs, s.ContactIDs = d.UserIDs, d.ContactIDs
	}
	if s.From == nil && s.To == nil {
		s.From, s.To = d.From, d.To
	}
	return s
}

// Retrieval tuning from the AI settings (§11.3).
type Tuning struct {
	EmbedModel    string  // vectors of other models are not compared; "" when AI is off
	MinSimilarity float64 // the relevance floor
	ExhaustiveMax int     // small sets skip ranking
}

// Found is what retrieval chose.
type Found struct {
	TicketIDs  []int64           // evidence, in rank order (or date order on the small-set path)
	Scores     map[int64]float64 // fused score per ranked ticket
	Closest    []int64           // up to five nearest tickets, for "Not enough information"
	Exhaustive bool              // the small-set path was taken
}

const (
	topChunks  = 50 // per list (§11.3)
	topTickets = 12
	rrfK       = 60
)

// Retrieve picks evidence in SQL under visibility and scope (§11.3). vec is
// the question's embedding, nil when AI is off: keyword search still runs.
// Tickets the question names by key come first when visible.
func Retrieve(ctx context.Context, pool *pgxpool.Pool, a Asker, s Scope, question string, vec []float32, keys []string, t Tuning) (Found, error) {
	q := db.New(pool)
	var found Found
	nodes := s.NodeIDs
	if len(nodes) > 0 {
		var err error
		if nodes, err = q.ExpandNodes(ctx, nodes); err != nil {
			return found, err
		}
	}
	from, to := dayBounds(s.From, s.To, a.TZ)
	f := filter{a.IsAdmin, a.UserID, orEmpty(s.ProjectIDs), orEmpty(nodes), orEmpty(s.ClientIDs), orEmpty(s.UserIDs), orEmpty(s.ContactIDs), from, to}

	// Chunks with all the words first; with fewer than 50 of those, chunks
	// with any of them.
	var kw []db.KeywordSearchRow
	words := keywordTerms(question)
	for i, op := range []string{" & ", " | "} {
		if len(words) == 0 || (i == 0 && len(words) == 1) {
			continue
		}
		rows, err := q.KeywordSearch(ctx, db.KeywordSearchParams{Terms: strings.Join(words, op), IsAdmin: f.admin, UserID: f.user, ProjectIds: f.projects,
			NodeIds: f.nodes, ClientIds: f.clients, UserIds: f.users, ContactIds: f.contacts, FromTs: f.from, ToTs: f.to, Candidates: keywordCandidates})
		if err != nil {
			return found, err
		}
		for _, r := range rows {
			if len(kw) < 50 && !slices.ContainsFunc(kw, func(k db.KeywordSearchRow) bool { return k.ID == r.ID }) {
				kw = append(kw, r)
			}
		}
		if len(kw) == 50 {
			break
		}
	}
	var vr []db.VectorSearchRow
	if vec != nil && t.EmbedModel != "" {
		var err error
		if vr, err = vectorSearch(ctx, pool, f, vec, t.EmbedModel); err != nil {
			return found, err
		}
	}

	// Reciprocal rank fusion over both lists; a ticket scores by its best chunk.
	chunkScore := map[int64]float64{}
	chunkTicket := map[int64]int64{}
	for i, r := range kw {
		chunkScore[r.ID] += 1.0 / float64(rrfK+i+1)
		chunkTicket[r.ID] = r.TicketID
	}
	best := 0.0
	for i, r := range vr {
		chunkScore[r.ID] += 1.0 / float64(rrfK+i+1)
		chunkTicket[r.ID] = r.TicketID
		best = max(best, r.Similarity)
	}
	found.Scores = map[int64]float64{}
	for id, sc := range chunkScore {
		if tk := chunkTicket[id]; sc > found.Scores[tk] {
			found.Scores[tk] = sc
		}
	}
	ranked := make([]int64, 0, len(found.Scores))
	for tk := range found.Scores {
		ranked = append(ranked, tk)
	}
	slices.SortFunc(ranked, func(x, y int64) int {
		if d := found.Scores[y] - found.Scores[x]; d != 0 {
			if d > 0 {
				return 1
			}
			return -1
		}
		return int(y - x) // newer tickets first on a tie
	})
	found.Closest = ranked[:min(5, len(ranked))]

	var named []int64
	if len(keys) > 0 {
		rows, err := q.VisibleTicketIDsByKey(ctx, db.VisibleTicketIDsByKeyParams{Keys: keys, IsAdmin: a.IsAdmin, UserID: a.UserID})
		if err != nil {
			return found, err
		}
		for _, r := range rows {
			named = append(named, r.ID)
		}
	}

	// The relevance floor: with no keyword hit and a best similarity below the
	// floor, there is no evidence and the chat model is not called (§11.3).
	if len(kw) == 0 && (len(vr) == 0 || best < t.MinSimilarity) {
		found.TicketIDs = named
		return found, nil
	}

	count, err := q.CountScopeTickets(ctx, db.CountScopeTicketsParams{IsAdmin: f.admin, UserID: f.user, ProjectIds: f.projects,
		NodeIds: f.nodes, ClientIds: f.clients, UserIds: f.users, ContactIds: f.contacts, FromTs: f.from, ToTs: f.to,
		Cap: int32(t.ExhaustiveMax) + 1})
	if err != nil {
		return found, err
	}
	pick := ranked[:min(topTickets, len(ranked))]
	if count > 0 && int(count) <= t.ExhaustiveMax {
		// A small set takes every item (§11.3): the ranked ones first, so the
		// evidence budget trims the least relevant, then the rest newest first.
		all, err := q.ListScopeTicketIDs(ctx, db.ListScopeTicketIDsParams{IsAdmin: f.admin, UserID: f.user, ProjectIds: f.projects,
			NodeIds: f.nodes, ClientIds: f.clients, UserIds: f.users, ContactIds: f.contacts, FromTs: f.from, ToTs: f.to,
			Lim: int32(t.ExhaustiveMax)})
		if err != nil {
			return found, err
		}
		pick = append([]int64(nil), ranked...)
		for _, id := range all {
			if !slices.Contains(pick, id) {
				pick = append(pick, id)
			}
		}
		found.Exhaustive = true
	}
	found.TicketIDs = named
	for _, id := range pick {
		if !slices.Contains(found.TicketIDs, id) {
			found.TicketIDs = append(found.TicketIDs, id)
		}
	}
	return found, nil
}

type filter struct {
	admin                                     bool
	user                                      int64
	projects, nodes, clients, users, contacts []int64
	from, to                                  *time.Time
}

// vectorSearch runs the HNSW search with iterative scans, so a narrow filter
// still returns up to 50 rows (§11.3).
func vectorSearch(ctx context.Context, pool *pgxpool.Pool, f filter, vec []float32, model string) ([]db.VectorSearchRow, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for _, set := range []string{"SET LOCAL hnsw.iterative_scan = relaxed_order", "SET LOCAL hnsw.ef_search = 100"} {
		if _, err := tx.Exec(ctx, set); err != nil {
			return nil, err
		}
	}
	return db.New(pool).WithTx(tx).VectorSearch(ctx, db.VectorSearchParams{
		Vec: pgvector.NewHalfVector(vec), Model: model, IsAdmin: f.admin, UserID: f.user, ProjectIds: f.projects,
		NodeIds: f.nodes, ClientIds: f.clients, UserIds: f.users, ContactIds: f.contacts, FromTs: f.from, ToTs: f.to,
	})
}

// dayBounds turns inclusive days into [from, to) instants in tz.
func dayBounds(from, to *time.Time, tz *time.Location) (*time.Time, *time.Time) {
	if tz == nil {
		tz = time.UTC
	}
	var f, t *time.Time
	if from != nil {
		d := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, tz)
		f = &d
	}
	if to != nil {
		d := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, tz).AddDate(0, 0, 1)
		t = &d
	}
	return f, t
}

// keywordCandidates bounds how many matching chunks one keyword search ranks.
// Ranking reads each row, so at 100,000 tickets a common word would otherwise
// cost seconds (FSD §18 load test).
const keywordCandidates = 5000

// keywordTerms lists the question's words, leaving out common words and one-letter words.
func keywordTerms(question string) []string {
	var terms []string
	for _, w := range tokenRe.FindAllString(strings.ToLower(question), -1) {
		if len([]rune(w)) < 2 || indonesianSW[w] || englishSW[w] || slices.Contains(terms, w) || strings.Contains(w, "-") {
			continue
		}
		terms = append(terms, w)
	}
	return terms
}

func orEmpty(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
