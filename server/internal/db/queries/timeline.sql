-- name: ListNodeTimeline :many
-- The visible tickets on these nodes (R-AC-3, R-AC-7) for a node page (FSD
-- §7.4): open ones first, newest first, then closed ones by close date, newest
-- first, each with its decision record. An entry's date is its close date, or
-- its creation date while open; from_date and to_date filter on it.
SELECT t.id, t.key, t.title, t.type, t.client_id, c.name AS client_name,
       t.requester_contact_id, rc.name AS requester_contact_name, rc.title AS requester_contact_title,
       t.requester_user_id, ru.name AS requester_user_name, t.created_at, t.closed_at, sqlc.embed(s),
       d.what_changed, d.why, d.alternatives, d.outcome, d.state, d.confirmed_by, cu.name AS confirmer_name, d.confirmed_at,
       sk.key AS superseded_by_key, t.accepted_on, ac.name AS accepted_contact_name, t.release_id
FROM tickets t
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
LEFT JOIN contacts ac ON ac.id = t.accepted_contact_id
LEFT JOIN contacts rc ON rc.id = t.requester_contact_id
LEFT JOIN users ru ON ru.id = t.requester_user_id
LEFT JOIN decision_records d ON d.ticket_id = t.id
LEFT JOIN users cu ON cu.id = d.confirmed_by
LEFT JOIN tickets sk ON sk.id = d.superseded_by
WHERE t.project_id = sqlc.arg('project_id')
  AND EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
  AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND (sqlc.narg('client_id')::bigint IS NULL OR t.client_id = sqlc.narg('client_id')::bigint)
  AND (NOT sqlc.arg('core_only')::boolean OR t.client_id IS NULL)
  AND (sqlc.narg('type')::text IS NULL OR t.type = sqlc.narg('type')::text)
  AND (sqlc.narg('from_date')::date IS NULL OR coalesce(t.closed_at, t.created_at) >= sqlc.narg('from_date')::date)
  AND (sqlc.narg('to_date')::date IS NULL OR coalesce(t.closed_at, t.created_at) < sqlc.narg('to_date')::date + 1)
ORDER BY t.closed_at IS NOT NULL, coalesce(t.closed_at, t.created_at) DESC, t.id DESC
LIMIT sqlc.arg('lim') OFFSET sqlc.arg('off');

-- name: ListNodeBehaviors :many
-- The decisions in force on these nodes: confirmed and implemented, on tickets
-- the scope may see; core work first, then by client, newest first (FSD §7.4).
-- A decision another ticket reverses is no longer in force (R-TK-5).
-- ponytail: at most 500.
-- Done tickets without a confirmed record, which come from imports, count too,
-- marked unconfirmed, with their title and reason (MSL-13).
SELECT b.key, b.title, b.client_id, b.client_name, b.closed_at, b.what_changed, b.why, b.alternatives, b.unconfirmed
FROM (
  SELECT t.id, t.key, t.title, t.client_id, c.name AS client_name, t.closed_at, d.what_changed, d.why, d.alternatives, false AS unconfirmed
  FROM tickets t
  JOIN decision_records d ON d.ticket_id = t.id
  LEFT JOIN clients c ON c.id = t.client_id
  WHERE t.project_id = sqlc.arg('project_id')
    AND d.state = 'confirmed' AND d.outcome = 'implemented' AND d.superseded_by IS NULL
    AND EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
    AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  UNION ALL
  SELECT t.id, t.key, t.title, t.client_id, c.name, t.closed_at, t.title, t.reason, '', true
  FROM tickets t
  JOIN statuses st ON st.id = t.status_id AND st.category = 'done'
  LEFT JOIN clients c ON c.id = t.client_id
  WHERE t.project_id = sqlc.arg('project_id')
    AND NOT EXISTS (SELECT 1 FROM decision_records d WHERE d.ticket_id = t.id AND d.state = 'confirmed')
    AND EXISTS (SELECT 1 FROM ticket_nodes tn WHERE tn.ticket_id = t.id AND tn.node_id = ANY (sqlc.arg('node_ids')::bigint[]))
    AND (sqlc.arg('all_clients')::boolean OR t.client_id IS NULL OR t.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
) b
ORDER BY b.client_id IS NOT NULL, lower(b.client_name), b.client_id, b.closed_at DESC, b.id DESC
LIMIT 500;
