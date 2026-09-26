package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/db"
)

// IndexSection rebuilds one document section's chunks (FSD §7.7, R-MR-13).
type IndexSection struct {
	SectionID int64 `json:"section_id"`
}

func (IndexSection) Kind() string { return "index_section" }

func (IndexSection) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, MaxAttempts: 10}
}

// SectionSource is one document section as its chunks and Ask's evidence describe it.
type SectionSource struct {
	Section db.GetSectionRow
	Menus   []db.ListSectionNodePathsRow
}

// Key is the section's citation key, e.g. HRIS-DOC1/7.4.
func (s SectionSource) Key() string { return s.Section.DocumentKey + "/" + s.Section.Number }

// LoadSection reads a section; one of an archived document has no chunks (pgx.ErrNoRows).
func LoadSection(ctx context.Context, q *db.Queries, sectionID int64) (SectionSource, error) {
	s, err := q.GetSection(ctx, sectionID)
	if err != nil {
		return SectionSource{}, err
	}
	if s.DocumentArchivedAt != nil {
		return SectionSource{}, pgx.ErrNoRows
	}
	src := SectionSource{Section: s}
	if src.Menus, err = q.ListSectionNodePaths(ctx, sectionID); err != nil {
		return SectionSource{}, err
	}
	return src, nil
}

// BuildSection turns a section into its chunks under a context line such as
// "HRIS-DOC1/7.4 · All clients · Overtime Approval". The filter columns are
// the document's project and client and the nodes the section produced; its
// date is the upload date.
func BuildSection(src SectionSource) []db.UpsertChunkParams {
	s := src.Section
	parts := []string{src.Key(), "All clients"}
	if s.ClientName != nil {
		parts[1] = *s.ClientName
	}
	nodeIDs := make([]int64, len(src.Menus))
	paths := make([]string, len(src.Menus))
	for i, m := range src.Menus {
		parts = append(parts, m.Path[len(m.Path)-1])
		nodeIDs[i] = m.ID
		paths[i] = strings.Join(m.Path, " › ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Document %s, section %s %s\n", s.DocumentTitle, s.Number, s.Title)
	if s.SupersededByKey != nil {
		fmt.Fprintf(&b, "Superseded by %s\n", *s.SupersededByKey)
	}
	if len(paths) > 0 {
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	b.WriteString(strings.TrimSpace(s.Body))
	line := strings.Join(parts, " · ")
	var out []db.UpsertChunkParams
	for i, part := range split(strings.TrimSpace(b.String())) {
		content := line + "\n" + part
		sum := sha256.Sum256([]byte(content))
		out = append(out, db.UpsertChunkParams{
			SourceType: "document", SourceID: s.ID, Seq: int32(i), SectionID: &s.ID, ProjectID: s.ProjectID, ClientID: s.ClientID,
			NodeIds: nodeIDs, UserIds: []int64{}, ContactIds: []int64{}, OccurredAt: s.DocumentCreatedAt, Content: content, ContentHash: sum[:],
		})
	}
	return out
}

// RebuildSection writes a section's chunks like Rebuild does a ticket's.
func (ix *Indexer) RebuildSection(ctx context.Context, sectionID int64) error {
	tx, err := ix.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := ix.q.WithTx(tx)
	if err := q.LockSectionIndex(ctx, sectionID); err != nil {
		return err
	}
	src, err := LoadSection(ctx, q, sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := q.DeleteSectionChunks(ctx, &sectionID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	chunks := BuildSection(src)
	keep := make([]string, len(chunks))
	for i, c := range chunks {
		if err := q.UpsertChunk(ctx, c); err != nil {
			return err
		}
		keep[i] = Key(c)
	}
	if err := q.DeleteStaleSectionChunks(ctx, db.DeleteStaleSectionChunksParams{SectionID: sectionID, Keep: keep}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EmbedSection embeds the section's chunks that lack a current vector.
func (ix *Indexer) EmbedSection(ctx context.Context, sectionID int64) error {
	return ix.embed(ctx, func(model string) ([]pending, error) {
		rows, err := ix.q.ListPendingSectionChunks(ctx, db.ListPendingSectionChunksParams{SectionID: sectionID, Model: model})
		out := make([]pending, len(rows))
		for i, r := range rows {
			out[i] = pending{r.ID, r.Content, r.ContentHash}
		}
		return out, err
	}, false)
}

type sectionWorker struct {
	river.WorkerDefaults[IndexSection]
	ix *Indexer
}

func (w *sectionWorker) Work(ctx context.Context, job *river.Job[IndexSection]) error {
	if err := w.ix.RebuildSection(ctx, job.Args.SectionID); err != nil {
		return err
	}
	return w.ix.EmbedSection(ctx, job.Args.SectionID)
}
