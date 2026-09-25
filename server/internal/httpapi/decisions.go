package httpapi

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/kenzo03/muasal/server/internal/db"
)

// decisionText is a decision record's words, trimmed.
type decisionText struct{ WhatChanged, Why, Alternatives string }

// checkDecision validates a decision record's words (FSD §9.1): what changed
// 10–1,000 characters, why 10–2,000, alternatives up to 2,000. prefix names the
// fields the way the request nests them.
func checkDecision(prefix string, in *DecisionInput) (decisionText, []FieldError) {
	var d decisionText
	if in != nil {
		d = decisionText{WhatChanged: strings.TrimSpace(in.WhatChanged), Why: strings.TrimSpace(in.Why), Alternatives: strings.TrimSpace(deref(in.Alternatives))}
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
	if d.ConfirmedBy != nil {
		out.ConfirmedBy = &Ref{Id: *d.ConfirmedBy, Name: deref(row.ConfirmerName)}
	}
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
