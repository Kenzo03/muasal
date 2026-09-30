package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/riverqueue/river"

	"github.com/kenzo03/muasal/server/internal/access"
	"github.com/kenzo03/muasal/server/internal/db"
	"github.com/kenzo03/muasal/server/internal/draft"
	"github.com/kenzo03/muasal/server/internal/indexer"
)

// ListSummarySchedules lists a project's weekly change summaries for its admins.
func (s *Server) ListSummarySchedules(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	rows, err := s.q.ListSummarySchedules(r.Context(), pc.project.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := SummaryScheduleList{Items: make([]SummarySchedule, len(rows))}
	for i, x := range rows {
		out.Items[i] = toAPISchedule(x.SummarySchedule, x.ClientName, x.CreatorName)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateSummarySchedule adds a weekly summary for a client of the project, or all.
func (s *Server) CreateSummarySchedule(w http.ResponseWriter, r *http.Request, key string) {
	pc, ok := s.projectFor(w, r, key, access.Admin)
	if !ok {
		return
	}
	var in SummaryScheduleCreate
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	var fields []FieldError
	if in.Weekday < 1 || in.Weekday > 7 {
		fields = append(fields, FieldError{Field: "weekday", Code: "invalid", Message: "Choose a weekday from 1 (Monday) to 7 (Sunday)"})
	}
	if in.Language != SummaryLanguageId && in.Language != SummaryLanguageEn {
		fields = append(fields, FieldError{Field: "language", Code: "invalid", Message: "Choose id or en"})
	}
	if in.Audience != SummaryAudienceInternal && in.Audience != SummaryAudienceClient {
		fields = append(fields, FieldError{Field: "audience", Code: "invalid", Message: "Choose internal or client"})
	}
	var clientName *string
	if in.ClientId != nil {
		linked, err := s.q.ListProjectClients(ctx, db.ListProjectClientsParams{ProjectID: pc.project.ID, AllClients: true, ClientIds: []int64{}})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if i := slices.IndexFunc(linked, func(c db.Client) bool { return c.ID == *in.ClientId }); i >= 0 {
			clientName = &linked[i].Name
		} else {
			fields = append(fields, FieldError{Field: "client_id", Code: "invalid", Message: "Choose a client of this project"})
		}
	}
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out SummarySchedule
	err := s.inTx(ctx, func(q *db.Queries) error {
		x, err := q.CreateSummarySchedule(ctx, db.CreateSummaryScheduleParams{
			ProjectID: pc.project.ID, ClientID: in.ClientId, Language: string(in.Language), Audience: string(in.Audience),
			Weekday: int16(in.Weekday), CreatedBy: pc.user.ID,
		})
		if err != nil {
			return err
		}
		out = toAPISchedule(x, clientName, pc.user.Name)
		return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "summary_schedule", x.ID, "create",
			map[string]any{"client": deref(clientName), "language": x.Language, "audience": x.Audience, "weekday": x.Weekday})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// DeleteSummarySchedule stops a weekly summary; the summaries it wrote stay.
func (s *Server) DeleteSummarySchedule(w http.ResponseWriter, r *http.Request, id int64) {
	u := s.requireUser(w, r)
	if u == nil {
		return
	}
	ctx := r.Context()
	x, err := s.q.GetSummarySchedule(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not_found", "Schedule not found")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, err := s.q.GetProjectByID(ctx, x.ProjectID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pc, ok := s.memberOf(w, r, u, p)
	if !ok {
		return
	}
	if !pc.scope.Allows(access.Admin) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Your project role does not allow this")
		return
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		if err := q.DeleteSummarySchedule(ctx, id); err != nil {
			return err
		}
		return audit(ctx, q, webMeta(r).inProject(p.ID), &u.ID, "summary_schedule", id, "delete",
			map[string]any{"language": x.Language, "audience": x.Audience, "weekday": x.Weekday})
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toAPISchedule(x db.SummarySchedule, clientName *string, creator string) SummarySchedule {
	out := SummarySchedule{Id: x.ID, Language: SummaryLanguage(x.Language), Audience: SummaryAudience(x.Audience),
		Weekday: int(x.Weekday), CreatedBy: Ref{Id: x.CreatedBy, Name: creator}}
	if x.ClientID != nil {
		out.Client = &Ref{Id: *x.ClientID, Name: deref(clientName)}
	}
	if x.LastRunOn != nil {
		out.LastRunOn = &openapi_types.Date{Time: *x.LastRunOn}
	}
	return out
}

// isoWeekday is t's ISO weekday: 1 is Monday, 7 is Sunday.
func isoWeekday(t time.Time) int16 { return int16((int(t.Weekday())+6)%7 + 1) }

// RunSummarySchedules writes the weekly summaries due today, by UTC date: the
// past seven days' closed tickets and decision notes, saved as the
// scheduler's summary with a bell notice. A week without changes writes none,
// nor does a scheduler who is no longer a project admin. Each schedule runs
// once a day, so a retry never writes one twice.
func (s *Server) RunSummarySchedules(ctx context.Context) error {
	now := s.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	due, err := s.q.ListDueSummarySchedules(ctx, db.ListDueSummarySchedulesParams{Weekday: isoWeekday(today), Today: today})
	if err != nil {
		return err
	}
	for _, sch := range due {
		if err := s.runSummarySchedule(ctx, sch, today); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) runSummarySchedule(ctx context.Context, sch db.SummarySchedule, today time.Time) error {
	ran := db.MarkSummaryScheduleRunParams{ID: sch.ID, Today: today}
	skip := func() error { return s.q.MarkSummaryScheduleRun(ctx, ran) }
	u, err := s.q.GetUserByID(ctx, sch.CreatedBy)
	if err != nil {
		return err
	}
	if u.DisabledAt != nil {
		return skip()
	}
	p, err := s.q.GetProjectByID(ctx, sch.ProjectID)
	if err != nil {
		return err
	}
	scope, member, err := access.ForProject(ctx, s.q, &u, p.ID)
	if err != nil {
		return err
	}
	if !member || !scope.Allows(access.Admin) {
		return skip()
	}
	pc := projectCtx{user: &u, project: p, scope: scope}
	nodes, err := s.visibleNodes(ctx, pc)
	if err != nil {
		return err
	}
	ids := make([]int64, len(nodes))
	names := map[int64]string{}
	for i, n := range nodes {
		ids[i], names[n.ID] = n.ID, n.Name
	}
	from, to := today.AddDate(0, 0, -7), today.AddDate(0, 0, -1)
	items, err := s.summaryItems(ctx, pc, ids, true, sch.ClientID, from, to, false, names)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return skip()
	}
	client := sch.Audience == string(SummaryAudienceClient)
	inputs, outItems, err := s.summaryInputs(ctx, items, client)
	if err != nil {
		return err
	}
	res, err := draft.Summarize(ctx, s.ai, inputs, sch.Language, client)
	if de := (*draft.Error)(nil); errors.As(err, &de) {
		res = plainSummary(items) // AI off or unavailable: the decision records themselves
	} else if err != nil {
		return err
	}
	clientName := ""
	if sch.ClientID != nil {
		c, err := s.q.GetClient(ctx, *sch.ClientID)
		if err != nil {
			return err
		}
		clientName = c.Name
	}
	title := draft.Title(p.Name, clientName, from, to, sch.Language)
	md := draft.Markdown(title, res, inputs, sch.Language, client)
	params, _ := json.Marshal(SummaryScope{ProjectKey: p.Key, ClientId: sch.ClientID, From: openapi_types.Date{Time: from},
		To: openapi_types.Date{Time: to}, Language: SummaryLanguage(sch.Language), Audience: SummaryAudience(sch.Audience)})
	itemsJSON, _ := json.Marshal(outItems)
	return s.inTx(ctx, func(q *db.Queries) error {
		row, err := q.CreateSummary(ctx, db.CreateSummaryParams{
			ProjectID: p.ID, CreatedBy: u.ID, Title: title, Params: params, Items: itemsJSON, Markdown: md, Model: res.Model,
		})
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"kind": "summary", "name": title, "link": fmt.Sprintf("/summaries/%d", row.ID)})
		if err := q.NotifyJobDone(ctx, db.NotifyJobDoneParams{UserID: u.ID, Payload: payload}); err != nil {
			return err
		}
		return q.MarkSummaryScheduleRun(ctx, ran)
	})
}

// plainSummary lists each change from its decision record, by menu and date,
// for a weekly summary written without the model.
func plainSummary(items []summaryItem) draft.Summary {
	var out draft.Summary
	section := map[string]int{}
	for _, it := range items {
		text, why := it.api.Title, ""
		if d := it.decision; d != nil && d.State != nil {
			if wc := strings.TrimSpace(deref(d.WhatChanged)); wc != "" {
				text = wc
			}
			why = strings.TrimSpace(deref(d.Why))
		}
		i, ok := section[it.api.Menu]
		if !ok {
			i = len(out.Sections)
			section[it.api.Menu] = i
			out.Sections = append(out.Sections, draft.Section{Menu: it.api.Menu})
		}
		out.Sections[i].Bullets = append(out.Sections[i].Bullets, draft.Bullet{Date: it.api.Date.Time, Text: text, Why: why, Keys: []string{it.api.Key}})
	}
	return out
}

// SummaryScheduleTick asks for the weekly summaries due now; `app serve` runs it hourly.
type SummaryScheduleTick struct{}

func (SummaryScheduleTick) Kind() string { return "summary_schedules" }

func (SummaryScheduleTick) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: indexer.QueueIndex, MaxAttempts: 3}
}

// SummaryScheduleWorker runs SummaryScheduleTick on the API's server, which holds the summary code and AI runtime.
type SummaryScheduleWorker struct {
	river.WorkerDefaults[SummaryScheduleTick]
	Server *Server
}

func (w *SummaryScheduleWorker) Work(ctx context.Context, _ *river.Job[SummaryScheduleTick]) error {
	return w.Server.RunSummarySchedules(ctx)
}

// Timeout leaves a slow local model time for several summaries.
func (w *SummaryScheduleWorker) Timeout(*river.Job[SummaryScheduleTick]) time.Duration {
	return 45 * time.Minute
}
