package httpapi

import (
	"cmp"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kenzo03/muasal/server/internal/ask"
	"github.com/kenzo03/muasal/server/internal/db"
)

// loggedEvidence is one row of ask_queries.evidence: a ticket or a note.
type loggedEvidence struct {
	TicketID  int64   `json:"ticket_id"`
	NoteID    int64   `json:"note_id"`
	SectionID int64   `json:"section_id"`
	Score     float64 `json:"score"`
}

func (l loggedEvidence) ref() ask.Ref {
	if l.NoteID != 0 {
		return ask.Ref{ID: l.NoteID, Kind: ask.KindNote}
	}
	if l.SectionID != 0 {
		return ask.Ref{ID: l.SectionID, Kind: ask.KindSection}
	}
	return ask.Ref{ID: l.TicketID}
}

// ListAskLog lists every question for system admins, newest first (FSD §15.4).
func (s *Server) ListAskLog(w http.ResponseWriter, r *http.Request, params ListAskLogParams) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	lim := min(cmp.Or(deref(params.Limit), 50), 100)
	p := db.ListAskLogParams{Slow: deref(params.Slow), Down: deref(params.Down), UserID: params.UserId, Before: params.Before, Lim: int32(lim) + 1}
	if params.Status != nil {
		p.Status = ptr(string(*params.Status))
	}
	rows, err := s.q.ListAskLog(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := AskLogPage{Items: []AskLogEntry{}}
	for i, row := range rows {
		if i == lim {
			out.NextBefore = &out.Items[lim-1].Id
			break
		}
		out.Items = append(out.Items, AskLogEntry{
			Id: row.ID, CreatedAt: row.CreatedAt, User: Ref{Id: row.UserID, Name: row.UserName}, Question: row.Question,
			Status: AskLogEntryStatus(row.Status), LlmCalled: row.LlmCalled, LatencyMs: intPtr(row.LatencyMs), Model: row.Model,
			EvidenceCount: int(row.EvidenceCount), Citations: int(row.Citations), Feedback: feedbackOf(row.Rating, row.Reasons, row.Comment),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// GetAskLogEntry opens one question with its exact scope, evidence and scores,
// answer and dropped claims, for debugging trust issues (§15.4).
func (s *Server) GetAskLogEntry(w http.ResponseWriter, r *http.Request, id int64) {
	if s.requireAdmin(w, r) == nil {
		return
	}
	ctx := r.Context()
	q, err := s.q.GetAskLogEntry(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Entry not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var logged []loggedEvidence
	scope := map[string]any{}
	claims := []AskClaim{}
	var dropped *map[string]any
	if err := errors.Join(json.Unmarshal(q.Evidence, &logged), json.Unmarshal(q.Scope, &scope), unmarshalIf(q.Answer, &claims)); err != nil {
		s.fail(w, r, err)
		return
	}
	if q.Dropped != nil {
		d := map[string]any{}
		if err := json.Unmarshal(q.Dropped, &d); err != nil {
			s.fail(w, r, err)
			return
		}
		dropped = &d
	}
	evidence := []AskLogEvidence{}
	for _, l := range logged {
		items, err := s.engine.ItemsFor(ctx, nil, []ask.Ref{l.ref()})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, it := range items { // none when the note was archived since
			e := AskLogEvidence{Kind: AskLogEvidenceKind(it.Kind), Key: it.Key, Title: it.Title, Client: it.Client, RequestedBy: it.RequestedBy, Date: it.Date, Status: it.Status, Closed: it.Closed}
			if l.Score > 0 {
				e.Score = ptr(float32(l.Score))
			}
			evidence = append(evidence, e)
		}
	}
	writeJSON(w, http.StatusOK, AskLogDetail{
		Id: q.ID, CreatedAt: q.CreatedAt, User: Ref{Id: q.UserID, Name: q.UserName}, Question: q.Question,
		Status: AskLogDetailStatus(q.Status), LlmCalled: q.LlmCalled, LatencyMs: intPtr(q.LatencyMs), Model: q.Model,
		EvidenceCount: int(q.EvidenceCount), Citations: int(q.Citations), ThreadId: q.ThreadID, Language: q.Lang,
		FirstClaimMs: intPtr(q.FirstClaimMs), Scope: scope, Evidence: evidence, Claims: claims, Dropped: dropped,
		Feedback: feedbackOf(q.Rating, q.Reasons, q.Comment),
	})
}

// ListAudit lists audit events for system admins, newest first (§15.4).
func (s *Server) ListAudit(w http.ResponseWriter, r *http.Request, params ListAuditParams) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	lim := min(cmp.Or(deref(params.Limit), 50), 200)
	p := auditFilter(u, params.ActorId, params.Entity, params.Action, params.From, params.To)
	p.Before, p.Lim = params.Before, int32(lim)+1
	rows, err := s.q.ListAudit(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := AuditPage{Items: []AuditEvent{}}
	for i, row := range rows {
		if i == lim {
			out.NextBefore = &out.Items[lim-1].Id
			break
		}
		ev, err := toAuditEvent(row)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		out.Items = append(out.Items, ev)
	}
	writeJSON(w, http.StatusOK, out)
}

// ExportAudit writes the filtered audit log as CSV (§15.4).
func (s *Server) ExportAudit(w http.ResponseWriter, r *http.Request, params ExportAuditParams) {
	u := s.requireAdmin(w, r)
	if u == nil {
		return
	}
	p := auditFilter(u, params.ActorId, params.Entity, params.Action, params.From, params.To)
	p.Lim = 100_000
	rows, err := s.q.ListAudit(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-`+time.Now().Format("20060102")+`.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "occurred_at", "actor", "via", "entity", "entity_id", "project", "action", "changes"})
	for _, row := range rows {
		_ = cw.Write([]string{
			strconv.FormatInt(row.ID, 10), row.OccurredAt.UTC().Format(time.RFC3339), deref(row.ActorName), row.Via, row.Entity,
			strconv.FormatInt(row.EntityID, 10), deref(row.ProjectKey), row.Action, string(row.Changes),
		})
	}
	cw.Flush()
}

// auditFilter turns the query's whole days, in the admin's timezone, into bounds.
func auditFilter(u *db.User, actor *int64, entity, action *string, from, to *openapi_types.Date) db.ListAuditParams {
	tz, err := time.LoadLocation(u.Timezone)
	if err != nil {
		tz = time.UTC
	}
	p := db.ListAuditParams{ActorID: actor, Entity: entity, Action: action}
	if from != nil {
		p.Since = ptr(time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, tz))
	}
	if to != nil {
		p.Until = ptr(time.Date(to.Year(), to.Month(), to.Day()+1, 0, 0, 0, 0, tz))
	}
	return p
}

func toAuditEvent(row db.ListAuditRow) (AuditEvent, error) {
	ev := AuditEvent{Id: row.ID, OccurredAt: row.OccurredAt, Via: row.Via, Entity: row.Entity, EntityId: row.EntityID,
		Project: row.ProjectKey, Action: row.Action, Changes: map[string]any{}}
	if row.ActorID != nil {
		ev.Actor = &Ref{Id: *row.ActorID, Name: deref(row.ActorName)}
	}
	return ev, json.Unmarshal(row.Changes, &ev.Changes)
}

func unmarshalIf(b []byte, v any) error {
	if b == nil {
		return nil
	}
	return json.Unmarshal(b, v)
}

func intPtr(v *int32) *int {
	if v == nil {
		return nil
	}
	return ptr(int(*v))
}
