package draft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/ai"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/docs"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// DraftTree drafts a module tree from a document in the background (R-MR-12).
type DraftTree struct {
	DraftID int64 `json:"draft_id"`
}

func (DraftTree) Kind() string { return "draft_tree" }

func (DraftTree) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 3}
}

// TreeWorker runs DraftTree; `app serve` registers it.
type TreeWorker struct {
	river.WorkerDefaults[DraftTree]
	Pool *pgxpool.Pool
	AI   *ai.Runtime
}

// Timeout allows a long FSD on a slow model (§7.7: 15 minutes on the dev laptop).
func (w *TreeWorker) Timeout(*river.Job[DraftTree]) time.Duration { return 45 * time.Minute }

func (w *TreeWorker) Work(ctx context.Context, job *river.Job[DraftTree]) error {
	err := RunTreeDraft(ctx, w.Pool, w.AI, job.Args.DraftID)
	var de *Error
	if errors.As(err, &de) && job.Attempt < job.MaxAttempts && de.Code != "ai_off" {
		return err // retried; the draft stays running
	}
	if err != nil {
		return failDraft(ctx, w.Pool, job.Args.DraftID, err)
	}
	return nil
}

// ContextBudget is the characters of document one call may carry, from the
// AI settings' context tokens less room for the prompt, the outline and the answer.
func ContextBudget(s ai.Settings) int {
	return max(int(float64(s.ContextTokens-1200)*3.5)-outlineChars, 2000)
}

// TreeParts is how many model calls a document's draft takes.
func TreeParts(s ai.Settings, sections []docs.Section) int {
	return len(docs.Parts(sections, ContextBudget(s)))
}

// RunTreeDraft proposes the tree for a running draft and marks it ready, then
// tells its starter (§8.10). With AI off, the proposal comes from the
// headings and no model is called (R-MR-11, AC-MR-10).
func RunTreeDraft(ctx context.Context, pool *pgxpool.Pool, rt *ai.Runtime, draftID int64) error {
	q := db.New(pool)
	d, err := q.GetTreeDraft(ctx, draftID)
	if err != nil {
		return err
	}
	if d.TreeDraft.Status != "running" {
		return nil
	}
	rows, err := q.ListSections(ctx, d.TreeDraft.DocumentID)
	if err != nil {
		return err
	}
	sections := make([]docs.Section, len(rows))
	for i, r := range rows {
		sections[i] = docs.Section{Number: r.Number, Title: r.Title, Level: int(r.Level), Body: r.Body}
	}
	var cands []docs.Candidate
	if d.TreeDraft.UsedAi {
		s, err := rt.Store.Get(ctx)
		if err != nil {
			return err
		}
		cands, err = ExtractTree(ctx, rt, d.DocumentTitle, docs.Parts(sections, ContextBudget(s)), func(n int) {
			_ = q.SetDraftProgress(ctx, db.SetDraftProgressParams{ID: draftID, DoneParts: int32(n)})
		})
		if err != nil {
			return err
		}
	} else {
		cands = docs.FromHeadings(sections)
	}
	existing, err := Tree(ctx, q, d.TreeDraft.ProjectID)
	if err != nil {
		return err
	}
	proposal, _ := json.Marshal(docs.Merge(cands, existing))
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		tq := q.WithTx(tx)
		if err := tq.FinishTreeDraft(ctx, db.FinishTreeDraftParams{ID: draftID, Status: "ready", Proposal: proposal}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"kind": "tree_draft", "name": d.DocumentTitle, "link": fmt.Sprintf("/tree-drafts/%d", draftID)})
		return tq.NotifyJobDone(ctx, db.NotifyJobDoneParams{UserID: d.TreeDraft.CreatedBy, Payload: payload})
	})
}

func failDraft(ctx context.Context, pool *pgxpool.Pool, draftID int64, cause error) error {
	code := "failed"
	var de *Error
	if errors.As(cause, &de) {
		code = de.Code
	}
	return db.New(pool).FinishTreeDraft(ctx, db.FinishTreeDraftParams{ID: draftID, Status: "failed", Proposal: []byte("[]"), Error: code})
}

// Tree lists the project's live nodes by path, for matching a proposal.
func Tree(ctx context.Context, q *db.Queries, projectID int64) ([]docs.Existing, error) {
	nodes, err := q.ListNodes(ctx, db.ListNodesParams{ProjectID: projectID, AllClients: true, ClientIds: []int64{}})
	if err != nil {
		return nil, err
	}
	byID := map[int64]db.ListNodesRow{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	out := make([]docs.Existing, 0, len(nodes))
	for _, n := range nodes {
		path := []string{n.Name}
		for p := n.ParentID; p != nil; p = byID[*p].ParentID {
			if _, ok := byID[*p]; !ok {
				break
			}
			path = append([]string{byID[*p].Name}, path...)
		}
		out = append(out, docs.Existing{ID: n.ID, Path: path})
	}
	return out, nil
}
