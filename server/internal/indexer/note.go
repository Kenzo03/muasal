package indexer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kenzo03/zettra/server/internal/db"
)

// IndexNote rebuilds one decision note's chunks (FSD §9.4, §13.1).
type IndexNote struct {
	NoteID int64 `json:"note_id"`
}

func (IndexNote) Kind() string { return "index_note" }

func (IndexNote) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueIndex, MaxAttempts: 10}
}

// NoteSource is one decision note as its chunks and Ask's evidence describe it.
type NoteSource struct {
	Note    db.GetNoteByIDRow
	Menus   []db.ListNoteNodePathsRow
	Tickets []db.ListNoteTicketsRow
}

// LoadNote reads a note; an archived note has no chunks (pgx.ErrNoRows).
func LoadNote(ctx context.Context, q *db.Queries, noteID int64) (NoteSource, error) {
	n, err := q.GetNoteByID(ctx, noteID)
	if err != nil {
		return NoteSource{}, err
	}
	if n.DecisionNote.ArchivedAt != nil {
		return NoteSource{}, pgx.ErrNoRows
	}
	src := NoteSource{Note: n}
	if src.Menus, err = q.ListNoteNodePaths(ctx, noteID); err != nil {
		return NoteSource{}, err
	}
	if src.Tickets, err = q.ListNoteTickets(ctx, noteID); err != nil {
		return NoteSource{}, err
	}
	return src, nil
}

// BuildNote turns a note into its chunks: title, date, attendees and body,
// under a context line such as "HRIS-DN7 · Client A · Overtime Approval". The
// filter columns are the note's project, client and menus; its date is the
// decision date (§13.1).
func BuildNote(src NoteSource) []db.UpsertChunkParams {
	n := src.Note.DecisionNote
	parts := []string{n.Key, "All clients"}
	if src.Note.ClientName != nil {
		parts[1] = *src.Note.ClientName
	}
	nodeIDs := make([]int64, len(src.Menus))
	paths := make([]string, len(src.Menus))
	for i, m := range src.Menus {
		parts = append(parts, m.Path[len(m.Path)-1])
		nodeIDs[i] = m.ID
		paths[i] = strings.Join(m.Path, " › ")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Decision note: %s\nDecided on %s", n.Title, day(n.DecidedOn))
	if s := strings.TrimSpace(n.Attendees); s != "" {
		b.WriteString(" with " + s)
	}
	b.WriteString("\n")
	if len(paths) > 0 {
		b.WriteString("Menus: " + strings.Join(paths, "; ") + "\n")
	}
	if len(src.Tickets) > 0 {
		keys := make([]string, len(src.Tickets))
		for i, t := range src.Tickets {
			keys[i] = t.Key
		}
		b.WriteString("Tickets: " + strings.Join(keys, ", ") + "\n")
	}
	b.WriteString(strings.TrimSpace(n.Body))
	line := strings.Join(parts, " · ")
	var out []db.UpsertChunkParams
	for i, part := range split(strings.TrimSpace(b.String())) {
		content := line + "\n" + part
		sum := sha256.Sum256([]byte(content))
		out = append(out, db.UpsertChunkParams{
			SourceType: "note", SourceID: n.ID, Seq: int32(i), NoteID: &n.ID, ProjectID: n.ProjectID, ClientID: n.ClientID,
			NodeIds: nodeIDs, UserIds: []int64{}, ContactIds: []int64{}, OccurredAt: n.DecidedOn, Content: content, ContentHash: sum[:],
		})
	}
	return out
}

// RebuildNote writes a note's chunks like Rebuild does a ticket's.
func (ix *Indexer) RebuildNote(ctx context.Context, noteID int64) error {
	tx, err := ix.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := ix.q.WithTx(tx)
	if err := q.LockNoteIndex(ctx, noteID); err != nil {
		return err
	}
	src, err := LoadNote(ctx, q, noteID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := q.DeleteNoteChunks(ctx, noteID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	chunks := BuildNote(src)
	keep := make([]string, len(chunks))
	for i, c := range chunks {
		if err := q.UpsertChunk(ctx, c); err != nil {
			return err
		}
		keep[i] = Key(c)
	}
	if err := q.DeleteStaleNoteChunks(ctx, db.DeleteStaleNoteChunksParams{NoteID: noteID, Keep: keep}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// EmbedNote embeds the note's chunks that lack a current vector.
func (ix *Indexer) EmbedNote(ctx context.Context, noteID int64) error {
	return ix.embed(ctx, func(model string) ([]pending, error) {
		rows, err := ix.q.ListPendingNoteChunks(ctx, db.ListPendingNoteChunksParams{NoteID: noteID, Model: model})
		out := make([]pending, len(rows))
		for i, r := range rows {
			out[i] = pending{r.ID, r.Content, r.ContentHash}
		}
		return out, err
	}, false)
}

type noteWorker struct {
	river.WorkerDefaults[IndexNote]
	ix *Indexer
}

func (w *noteWorker) Work(ctx context.Context, job *river.Job[IndexNote]) error {
	if err := w.ix.RebuildNote(ctx, job.Args.NoteID); err != nil {
		return err
	}
	return w.ix.EmbedNote(ctx, job.Args.NoteID)
}
