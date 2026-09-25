-- name: ListScopeClients :many
-- Live clients the asker may see in some project, so scope detection never
-- names a hidden client (FSD §11.2, R-AC-1).
SELECT c.id, c.name, c.code, c.aliases
FROM clients c
WHERE c.archived_at IS NULL
  AND (sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM project_clients pc
        JOIN memberships m ON m.project_id = pc.project_id AND m.user_id = sqlc.arg('user_id')::bigint
        WHERE pc.client_id = c.id
          AND (m.all_clients OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = c.id))))
ORDER BY c.id;

-- name: ListScopeNodes :many
-- Live nodes the asker may see, with their parent, for "the deepest match
-- wins" (§11.2). Same visibility as search (R-AC-5).
WITH RECURSIVE visible AS (
  SELECT n.id, 0 AS depth
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
  SELECT n.id, v.depth + 1
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
SELECT n.id, n.parent_id, n.project_id, n.name, n.code, n.aliases, v.depth::int AS depth
FROM visible v
JOIN nodes n ON n.id = v.id
ORDER BY n.id;

-- name: ListScopePeople :many
-- People the asker may name: users who share a project with them, and
-- contacts that are internal or belong to a client they see (§11.2).
SELECT 'user'::text AS kind, u.id, u.name
FROM users u
WHERE sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM memberships me JOIN memberships them ON them.project_id = me.project_id
        WHERE me.user_id = sqlc.arg('user_id')::bigint AND them.user_id = u.id)
UNION ALL
SELECT 'contact'::text, ct.id, ct.name
FROM contacts ct
WHERE ct.client_id IS NULL OR sqlc.arg('is_admin')::boolean OR EXISTS (
        SELECT 1 FROM project_clients pc
        JOIN memberships m ON m.project_id = pc.project_id AND m.user_id = sqlc.arg('user_id')::bigint
        WHERE pc.client_id = ct.client_id
          AND (m.all_clients OR EXISTS (
                SELECT 1 FROM membership_clients mc
                WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = ct.client_id)))
ORDER BY 1, 2;
