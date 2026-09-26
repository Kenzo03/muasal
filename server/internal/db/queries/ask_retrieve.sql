-- The Ask engine's retrieval (FSD §11.3). Every query applies the visibility
-- predicate to the chunk's project and client (§5.3, R-AC-7) and the scope in
-- SQL, never after. An empty array means "no filter"; node_ids arrive already
-- expanded to sub-nodes; dates filter on the item's date, the close date or,
-- while open, the creation date (§10.2), with to_ts exclusive.

-- name: ExpandNodes :many
-- The nodes and all their sub-nodes, archived ones too, so history under an
-- archived menu still counts.
WITH RECURSIVE sub AS (
  SELECT n.id FROM nodes n WHERE n.id = ANY (sqlc.arg('node_ids')::bigint[])
  UNION
  SELECT n.id FROM nodes n JOIN sub ON n.parent_id = sub.id
)
SELECT id FROM sub ORDER BY id;

-- name: CountScopeTickets :one
-- How many tickets are in scope, counting no further than cap: retrieval only
-- asks whether the set is small (§11.3). Driven from tickets with LIMIT, it
-- stops early on a broad scope instead of scanning every chunk.
SELECT count(*)
FROM (
  SELECT t.id
  FROM tickets t
  WHERE EXISTS (
    SELECT 1 FROM chunks ch
    WHERE ch.ticket_id = t.id
      AND (sqlc.arg('is_admin')::boolean OR EXISTS (
            SELECT 1 FROM memberships m
            WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
              AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                    SELECT 1 FROM membership_clients mc
                    WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
      AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
      AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
      AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
      AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
           OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[]))
    AND (sqlc.narg('from_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at) >= sqlc.narg('from_ts')::timestamptz)
    AND (sqlc.narg('to_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at) < sqlc.narg('to_ts')::timestamptz)
  LIMIT sqlc.arg('cap')
) scoped;

-- name: ListScopeTicketIDs :many
-- Every ticket in scope, newest first by item date: the small-set path.
SELECT t.id
FROM tickets t
WHERE EXISTS (
  SELECT 1 FROM chunks ch
  WHERE ch.ticket_id = t.id
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
            AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                  SELECT 1 FROM membership_clients mc
                  WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
    AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
    AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
    AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
    AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
         OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[]))
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at) >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at) < sqlc.narg('to_ts')::timestamptz)
ORDER BY coalesce(t.closed_at, t.created_at) DESC, t.id DESC
LIMIT sqlc.arg('lim');

-- name: VectorSearch :many
-- The 50 chunks nearest the question, among chunks embedded by the current
-- model. A chunk belongs to a ticket or to a decision note (§9.4); a note's
-- date is its decision date. The caller sets hnsw.iterative_scan and hnsw.ef_search for its
-- transaction, so filtered searches still find enough rows (§11.3).
SELECT ch.id, ch.ticket_id, ch.note_id, ch.section_id, (1 - (ch.embedding <=> sqlc.arg('vec')::halfvec))::float8 AS similarity
FROM chunks ch
LEFT JOIN tickets t ON t.id = ch.ticket_id
WHERE ch.embed_model = sqlc.arg('model')::text
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
          AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
  AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
  AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
  AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
       OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[])
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at, ch.occurred_at) >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at, ch.occurred_at) < sqlc.narg('to_ts')::timestamptz)
ORDER BY ch.embedding <=> sqlc.arg('vec')::halfvec
LIMIT 50;

-- name: KeywordSearch :many
-- The 50 chunks whose words best match the question. The 'simple'
-- configuration skips stemming, which suits mixed Indonesian-English text,
-- IDs and names (§11.3). Ranking reads every candidate row, so it ranks at
-- most `candidates` of them: a word found in thousands of chunks says little
-- on its own, and the caller tries all the words together first.
SELECT c.id, c.ticket_id, c.note_id, c.section_id, c.rank
FROM (
SELECT ch.id, ch.ticket_id, ch.note_id, ch.section_id, ts_rank_cd(ch.tsv, query)::float8 AS rank
FROM chunks ch
LEFT JOIN tickets t ON t.id = ch.ticket_id,
     to_tsquery('simple', sqlc.arg('terms')::text) AS query
WHERE ch.tsv @@ query
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
          AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
  AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
  AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
  AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
  AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
       OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[])
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at, ch.occurred_at) >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR coalesce(t.closed_at, t.created_at, ch.occurred_at) < sqlc.narg('to_ts')::timestamptz)
LIMIT sqlc.arg('candidates')
) c
ORDER BY c.rank DESC, c.id
LIMIT 50;

-- name: VisibleTicketIDsByKey :many
-- Tickets a question names by key that the asker may open (§11.2).
SELECT t.id, t.key
FROM tickets t
WHERE t.key = ANY (sqlc.arg('keys')::text[])
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))));

-- name: VisibleTicketIDs :many
-- Of the given tickets, those the asker may open now, for re-showing a
-- thread's evidence (§10.6).
SELECT t.id
FROM tickets t
WHERE t.id = ANY (sqlc.arg('ids')::bigint[])
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))));

-- name: CountScopeNotes :one
-- How many decision notes are in scope, counting no further than cap (§11.3).
SELECT count(*)
FROM (
  SELECT n.id
  FROM decision_notes n
  WHERE n.archived_at IS NULL
    AND EXISTS (
    SELECT 1 FROM chunks ch
    WHERE ch.note_id = n.id
      AND (sqlc.arg('is_admin')::boolean OR EXISTS (
            SELECT 1 FROM memberships m
            WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
              AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                    SELECT 1 FROM membership_clients mc
                    WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
      AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
      AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
      AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
      AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
           OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[]))
    AND (sqlc.narg('from_ts')::timestamptz IS NULL OR n.decided_on >= sqlc.narg('from_ts')::timestamptz)
    AND (sqlc.narg('to_ts')::timestamptz IS NULL OR n.decided_on < sqlc.narg('to_ts')::timestamptz)
  LIMIT sqlc.arg('cap')
) scoped;

-- name: ListScopeNoteIDs :many
-- Every decision note in scope, newest first by decision date: the small-set path.
SELECT n.id
FROM decision_notes n
WHERE n.archived_at IS NULL
  AND EXISTS (
  SELECT 1 FROM chunks ch
  WHERE ch.note_id = n.id
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = ch.project_id
            AND (m.all_clients OR ch.client_id IS NULL OR EXISTS (
                  SELECT 1 FROM membership_clients mc
                  WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ch.client_id))))
    AND (cardinality(sqlc.arg('project_ids')::bigint[]) = 0 OR ch.project_id = ANY (sqlc.arg('project_ids')::bigint[]))
    AND (cardinality(sqlc.arg('node_ids')::bigint[]) = 0 OR ch.node_ids && sqlc.arg('node_ids')::bigint[])
    AND (cardinality(sqlc.arg('client_ids')::bigint[]) = 0 OR ch.client_id IS NULL OR ch.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
    AND ((cardinality(sqlc.arg('user_ids')::bigint[]) = 0 AND cardinality(sqlc.arg('contact_ids')::bigint[]) = 0)
         OR ch.user_ids && sqlc.arg('user_ids')::bigint[] OR ch.contact_ids && sqlc.arg('contact_ids')::bigint[]))
  AND (sqlc.narg('from_ts')::timestamptz IS NULL OR n.decided_on >= sqlc.narg('from_ts')::timestamptz)
  AND (sqlc.narg('to_ts')::timestamptz IS NULL OR n.decided_on < sqlc.narg('to_ts')::timestamptz)
ORDER BY n.decided_on DESC, n.id DESC
LIMIT sqlc.arg('lim');

-- name: VisibleNoteIDsByKey :many
-- Notes a question names by key that the asker may open (§11.2).
SELECT n.id, n.key
FROM decision_notes n
WHERE n.key = ANY (sqlc.arg('keys')::text[]) AND n.archived_at IS NULL
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
          AND (m.all_clients OR n.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = n.client_id))));

-- name: VisibleNoteIDs :many
-- Of the given notes, those the asker may open now (§10.6).
SELECT n.id
FROM decision_notes n
WHERE n.id = ANY (sqlc.arg('ids')::bigint[]) AND n.archived_at IS NULL
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
          AND (m.all_clients OR n.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = n.client_id))));
