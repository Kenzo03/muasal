-- name: ListNodes :many
-- The project's live tree as a flat list, siblings in position order. A
-- client-specific menu, and everything under it, is left out unless one of its
-- clients is in scope (R-AC-5); client_ids and client_names hold only in-scope
-- clients (R-MR-8). UNION, not UNION ALL, so a cycle could never loop forever.
WITH RECURSIVE visible AS (
  SELECT n.id FROM nodes n
  WHERE n.project_id = sqlc.arg('project_id') AND n.parent_id IS NULL AND n.archived_at IS NULL
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
  UNION
  SELECT n.id FROM nodes n
  JOIN visible v ON n.parent_id = v.id
  WHERE n.archived_at IS NULL
    AND (NOT n.client_specific OR sqlc.arg('all_clients')::boolean
         OR EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = n.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[])))
)
SELECT n.id, n.parent_id, n.type, n.name, n.code, n.aliases, n.description, n.client_specific, n.position,
       coalesce(array_agg(c.id ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::bigint[] AS client_ids,
       coalesce(array_agg(c.name ORDER BY lower(c.name), c.id) FILTER (WHERE c.id IS NOT NULL), '{}')::text[] AS client_names
FROM visible v
JOIN nodes n ON n.id = v.id
LEFT JOIN node_clients nc ON nc.node_id = n.id
  AND (sqlc.arg('all_clients')::boolean OR nc.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
LEFT JOIN clients c ON c.id = nc.client_id
GROUP BY n.id
ORDER BY n.parent_id NULLS FIRST, n.position, n.id;

-- name: IsNodeVisible :one
-- Whether the node and every node above it are visible to this scope (R-AC-5).
WITH RECURSIVE up AS (
  SELECT s.id, s.parent_id, s.client_specific FROM nodes s WHERE s.id = sqlc.arg('id')
  UNION
  SELECT n.id, n.parent_id, n.client_specific FROM nodes n JOIN up ON n.id = up.parent_id
)
SELECT NOT EXISTS (
  SELECT 1 FROM up
  WHERE up.client_specific AND NOT sqlc.arg('all_clients')::boolean
    AND NOT EXISTS (SELECT 1 FROM node_clients nc
                    WHERE nc.node_id = up.id AND nc.client_id = ANY (sqlc.arg('client_ids')::bigint[]))
);

-- name: GetNode :one
SELECT * FROM nodes WHERE id = $1;

-- name: CreateNode :one
-- New nodes go last among their siblings.
INSERT INTO nodes (project_id, parent_id, type, name, code, aliases, description, client_specific, position)
VALUES (sqlc.arg('project_id'), sqlc.narg('parent_id'), sqlc.arg('type'), sqlc.arg('name'), sqlc.narg('code'),
        coalesce(sqlc.narg('aliases')::text[], '{}'), sqlc.arg('description'), sqlc.arg('client_specific'),
        (SELECT coalesce(max(s.position) + 1, 0) FROM nodes s
         WHERE s.project_id = sqlc.arg('project_id')
           AND s.parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::bigint))
RETURNING *;

-- name: UpdateNode :one
-- NULL keeps a field; an empty code clears it.
UPDATE nodes SET
  name            = coalesce(sqlc.narg('name'), name),
  type            = coalesce(sqlc.narg('type'), type),
  code            = CASE WHEN sqlc.narg('code')::text IS NULL THEN code ELSE nullif(sqlc.narg('code')::text, '') END,
  aliases         = coalesce(sqlc.narg('aliases')::text[], aliases),
  description     = coalesce(sqlc.narg('description'), description),
  client_specific = coalesce(sqlc.narg('client_specific'), client_specific)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: ListNodeClients :many
SELECT c.id, c.name FROM node_clients nc
JOIN clients c ON c.id = nc.client_id
WHERE nc.node_id = $1
ORDER BY lower(c.name), c.id;

-- name: ClearNodeClients :exec
DELETE FROM node_clients WHERE node_id = $1;

-- name: AddNodeClients :exec
INSERT INTO node_clients (node_id, project_id, client_id)
SELECT sqlc.arg('node_id')::bigint, sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[]);

-- name: IsSelfOrDescendant :one
-- Whether candidate is the node itself or anywhere below it: moving there would build a cycle.
WITH RECURSIVE below AS (
  SELECT s.id FROM nodes s WHERE s.id = sqlc.arg('node_id')
  UNION
  SELECT n.id FROM nodes n JOIN below b ON n.parent_id = b.id
)
SELECT EXISTS (SELECT 1 FROM below WHERE below.id = sqlc.arg('candidate_id')::bigint);

-- name: ListSiblingIDs :many
SELECT id FROM nodes
WHERE project_id = sqlc.arg('project_id')
  AND parent_id IS NOT DISTINCT FROM sqlc.narg('parent_id')::bigint
  AND id <> sqlc.arg('exclude_id') AND archived_at IS NULL
ORDER BY position, id;

-- name: PlaceNodes :exec
-- Puts the listed nodes, in this order, under one parent.
UPDATE nodes
SET parent_id = sqlc.narg('parent_id')::bigint,
    position  = array_position(sqlc.arg('ids')::bigint[], id) - 1
WHERE id = ANY (sqlc.arg('ids')::bigint[]);

-- name: DeleteNode :exec
DELETE FROM nodes WHERE id = $1;
