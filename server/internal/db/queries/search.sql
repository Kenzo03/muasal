-- name: SearchTickets :many
-- Tickets the user may see in any project (R-AC-2, R-AC-3): a key, words
-- anywhere in the ticket, or part of the title (FSD §6.1). An exact key comes
-- first, then title matches, then the latest updates.
-- ponytail: at most 50 results; pages come when people ask for them.
SELECT t.key, t.title, p.key AS project_key, t.client_id, c.name AS client_name, sqlc.embed(s)
FROM tickets t
JOIN projects p ON p.id = t.project_id
JOIN statuses s ON s.id = t.status_id
LEFT JOIN clients c ON c.id = t.client_id
WHERE (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships m
        WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = t.project_id
          AND (m.all_clients OR t.client_id IS NULL OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = t.client_id))))
  AND (t.key = upper(sqlc.arg('q')::text)
       OR t.title ILIKE '%' || sqlc.arg('q')::text || '%'
       OR to_tsvector('simple', t.key || ' ' || t.title || ' ' || t.reason || ' ' || t.description)
          @@ websearch_to_tsquery('simple', sqlc.arg('q')::text))
ORDER BY t.key = upper(sqlc.arg('q')::text) DESC, t.title ILIKE '%' || sqlc.arg('q')::text || '%' DESC, t.updated_at DESC
LIMIT 50;

-- name: SearchNodes :many
-- Live nodes the user may see (R-AC-5) whose name, alias or code holds q, with
-- the names on their path from the top of the tree. A client-specific menu,
-- and everything under it, needs one of its clients in the user's scope.
-- UNION, not UNION ALL, so a cycle could never loop forever.
WITH RECURSIVE visible AS (
  SELECT n.id, ARRAY[n.name]::text[] AS path
  FROM nodes n
  WHERE n.parent_id IS NULL AND n.archived_at IS NULL
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
            AND (NOT n.client_specific OR m.all_clients OR EXISTS (
                  SELECT 1 FROM node_clients nc
                  JOIN membership_clients mc ON mc.client_id = nc.client_id AND mc.project_id = m.project_id AND mc.user_id = m.user_id
                  WHERE nc.node_id = n.id))))
  UNION
  SELECT n.id, v.path || n.name
  FROM nodes n
  JOIN visible v ON n.parent_id = v.id
  WHERE n.archived_at IS NULL
    AND (sqlc.arg('is_admin')::boolean OR EXISTS (
          SELECT 1 FROM memberships m
          WHERE m.user_id = sqlc.arg('user_id')::bigint AND m.project_id = n.project_id
            AND (NOT n.client_specific OR m.all_clients OR EXISTS (
                  SELECT 1 FROM node_clients nc
                  JOIN membership_clients mc ON mc.client_id = nc.client_id AND mc.project_id = m.project_id AND mc.user_id = m.user_id
                  WHERE nc.node_id = n.id))))
)
SELECT n.id, n.name, n.type, n.code, n.aliases, p.key AS project_key, v.path::text[] AS path
FROM visible v
JOIN nodes n ON n.id = v.id
JOIN projects p ON p.id = n.project_id
WHERE n.name ILIKE '%' || sqlc.arg('q')::text || '%'
   OR n.code ILIKE '%' || sqlc.arg('q')::text || '%'
   OR array_to_string(n.aliases, ' ') ILIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY lower(n.name) = lower(sqlc.arg('q')::text) DESC, p.key, v.path
LIMIT 20;
