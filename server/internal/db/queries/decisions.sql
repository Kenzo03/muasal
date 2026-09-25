-- name: GetDecision :one
SELECT sqlc.embed(d), u.name AS confirmer_name
FROM decision_records d
LEFT JOIN users u ON u.id = d.confirmed_by
WHERE d.ticket_id = $1;

-- name: ConfirmDecision :one
-- A close writes the record, or confirms a reopened ticket's draft again, with
-- the closing user as confirmer (R-DC-4, R-DC-6).
INSERT INTO decision_records (ticket_id, what_changed, why, alternatives, outcome, state, confirmed_by, confirmed_at)
VALUES (sqlc.arg('ticket_id')::bigint, sqlc.arg('what_changed')::text, sqlc.arg('why')::text, sqlc.arg('alternatives')::text,
        sqlc.arg('outcome')::text, 'confirmed', sqlc.arg('confirmed_by')::bigint, now())
ON CONFLICT (ticket_id) DO UPDATE SET
  what_changed = excluded.what_changed, why = excluded.why, alternatives = excluded.alternatives,
  outcome = excluded.outcome, state = 'confirmed', confirmed_by = excluded.confirmed_by, confirmed_at = excluded.confirmed_at
RETURNING *;

-- name: DraftDecision :exec
-- A reopen turns the record back into a draft; the history keeps who confirmed it (R-DC-6).
UPDATE decision_records SET state = 'draft', confirmed_by = NULL, confirmed_at = NULL WHERE ticket_id = $1;

-- name: UpdateDecision :exec
UPDATE decision_records SET what_changed = sqlc.arg('what_changed'), why = sqlc.arg('why'), alternatives = sqlc.arg('alternatives')
WHERE ticket_id = sqlc.arg('ticket_id');
