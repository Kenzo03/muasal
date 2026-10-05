package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/zettra/server/internal/access"
	"github.com/kenzo03/zettra/server/internal/db"
)

// decisionText is a decision record's words, trimmed.
type decisionText struct {
	WhatChanged, Why, Alternatives string
	AIDrafted                      bool
}

// checkDecision validates a decision record's words (FSD §9.1): what changed
// 10–1,000 characters, why 10–2,000, alternatives up to 2,000. prefix names the
// fields the way the request nests them.
func checkDecision(prefix string, in *DecisionInput) (decisionText, []FieldError) {
	var d decisionText
	if in != nil {
		d = decisionText{WhatChanged: strings.TrimSpace(in.WhatChanged), Why: strings.TrimSpace(in.Why), Alternatives: strings.TrimSpace(deref(in.Alternatives)), AIDrafted: deref(in.AiDrafted)}
	}
	f := textRange(prefix+"what_changed", d.WhatChanged, 10, 1000, "Say what changed in 10 to 1,000 characters")
	f = append(f, textRange(prefix+"why", d.Why, 10, 2000, "Say why in 10 to 2,000 characters")...)
	if utf8.RuneCountInString(d.Alternatives) > 2000 {
		f = append(f, FieldError{Field: prefix + "alternatives", Code: "invalid", Message: "Use at most 2,000 characters"})
	}
	return d, f
}

// closeFieldErrors checks what a closed ticket needs of its own fields (TK-3):
// a reason of 10–2,000 characters and at least one menu (FSD §9.1).
func closeFieldErrors(reason string, menus int) []FieldError {
	f := textRange("reason", reason, 10, 2000, "Reason is required to close: 10 to 2,000 characters")
	if menus == 0 {
		f = append(f, FieldError{Field: "node_ids", Code: "min_items", Message: "Link at least one menu or module"})
	}
	return f
}

// textRange checks a required text: empty is "required", and a length outside
// lo–hi characters is "invalid".
func textRange(field, s string, lo, hi int, msg string) []FieldError {
	switch n := utf8.RuneCountInString(s); {
	case n == 0:
		return []FieldError{{Field: field, Code: "required", Message: msg}}
	case n < lo || n > hi:
		return []FieldError{{Field: field, Code: "invalid", Message: msg}}
	}
	return nil
}

// decisionOf reads a ticket's decision record; nil when it has none.
func decisionOf(ctx context.Context, q *db.Queries, ticketID int64) (*DecisionRecord, error) {
	row, err := q.GetDecision(ctx, ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ptr(toAPIDecision(row)), nil
}

func toAPIDecision(row db.GetDecisionRow) DecisionRecord {
	d := row.DecisionRecord
	out := DecisionRecord{
		WhatChanged: d.WhatChanged, Why: d.Why, Alternatives: d.Alternatives,
		Outcome: DecisionOutcome(d.Outcome), State: DecisionState(d.State), ConfirmedAt: d.ConfirmedAt,
	}
	if d.AiDrafted {
		out.AiDrafted = ptr(true)
	}
	if d.ConfirmedBy != nil {
		out.ConfirmedBy = &Ref{Id: *d.ConfirmedBy, Name: deref(row.ConfirmerName)}
	}
	out.SupersededBy = row.SupersededByKey
	return out
}

// decisionAudit is what the ticket's history keeps of its decision record, so
// every earlier version stays readable (R-DC-6).
func decisionAudit(d *DecisionRecord) map[string]any {
	if d == nil {
		return map[string]any{}
	}
	m := map[string]any{
		"what_changed": d.WhatChanged, "why": d.Why, "alternatives": d.Alternatives,
		"outcome": string(d.Outcome), "state": string(d.State), "confirmed_by": nil,
	}
	if d.ConfirmedBy != nil {
		m["confirmed_by"] = d.ConfirmedBy.Name
	}
	return m
}

// UpdateDecision rewords a confirmed decision record. Project admins and the
// confirmer may; every edit keeps the old words in the ticket's history (R-DC-5).
func (s *Server) UpdateDecision(w http.ResponseWriter, r *http.Request, key string) {
	pc, row, ok := s.ticketFor(w, r, key, access.Member)
	if !ok {
		return
	}
	var in DecisionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	before, err := decisionOf(ctx, s.q, row.Ticket.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if before == nil || before.State != DecisionStateConfirmed {
		writeProblem(w, http.StatusConflict, "decision_not_confirmed", "Close the ticket to confirm its decision record first")
		return
	}
	if !pc.scope.Allows(access.Admin) && (before.ConfirmedBy == nil || before.ConfirmedBy.Id != pc.user.ID) {
		writeProblem(w, http.StatusForbidden, "forbidden", "Only project admins and the confirmer edit a decision record")
		return
	}
	text, fields := checkDecision("", &in)
	if len(fields) > 0 {
		writeProblem(w, http.StatusUnprocessableEntity, "validation_failed", "Check the highlighted fields", fields...)
		return
	}
	var out *DecisionRecord
	err = s.inJobTx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		if err := q.UpdateDecision(ctx, db.UpdateDecisionParams{
			TicketID: row.Ticket.ID, WhatChanged: text.WhatChanged, Why: text.Why, Alternatives: text.Alternatives,
		}); err != nil {
			return err
		}
		var err error
		if out, err = decisionOf(ctx, q, row.Ticket.ID); err != nil {
			return err
		}
		if err := s.index(ctx, tx, row.Ticket.ID); err != nil {
			return err
		}
		if d := changed(decisionAudit(before), decisionAudit(out)); len(d) > 0 {
			return audit(ctx, q, webMeta(r).inProject(pc.project.ID), &pc.user.ID, "ticket", row.Ticket.ID, "decision_edit", d)
		}
		return nil
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
