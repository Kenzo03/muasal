-- name: GetMembership :one
SELECT m.role, m.all_clients,
       coalesce(array_agg(mc.client_id ORDER BY mc.client_id) FILTER (WHERE mc.client_id IS NOT NULL), '{}')::bigint[] AS client_ids
FROM memberships m
LEFT JOIN membership_clients mc ON mc.user_id = m.user_id AND mc.project_id = m.project_id
WHERE m.user_id = $1 AND m.project_id = $2
GROUP BY m.role, m.all_clients;

-- name: IsProjectAdminAnywhere :one
SELECT EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND role = 'admin');

-- name: ListProjectMembers :many
SELECT u.id AS user_id, u.name, u.email, m.role, m.all_clients,
       coalesce(array_agg(mc.client_id ORDER BY mc.client_id) FILTER (WHERE mc.client_id IS NOT NULL), '{}')::bigint[] AS client_ids
FROM memberships m
JOIN users u ON u.id = m.user_id
LEFT JOIN membership_clients mc ON mc.user_id = m.user_id AND mc.project_id = m.project_id
WHERE m.project_id = $1
GROUP BY u.id, u.name, u.email, m.role, m.all_clients
ORDER BY lower(u.name), u.id;

-- name: DeleteMembershipsExcept :exec
DELETE FROM memberships
WHERE project_id = sqlc.arg('project_id') AND NOT (user_id = ANY (sqlc.arg('user_ids')::bigint[]));

-- name: UpsertMembership :exec
INSERT INTO memberships (user_id, project_id, role, all_clients)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, project_id) DO UPDATE SET role = excluded.role, all_clients = excluded.all_clients;

-- name: ClearMembershipClients :exec
DELETE FROM membership_clients WHERE user_id = $1 AND project_id = $2;

-- name: AddMembershipClients :exec
INSERT INTO membership_clients (user_id, project_id, client_id)
SELECT sqlc.arg('user_id')::bigint, sqlc.arg('project_id')::bigint, unnest(sqlc.arg('client_ids')::bigint[]);

-- name: ListAssignees :many
-- Who can own tickets: active members who are not viewers (FSD §8.1). With a
-- client, only those who may see its tickets (MSL-22): project admins, members
-- of all clients, and members scoped to it.
SELECT u.id, u.name
FROM memberships m JOIN users u ON u.id = m.user_id
WHERE m.project_id = sqlc.arg('project_id') AND m.role IN ('admin', 'member') AND u.disabled_at IS NULL
  AND (sqlc.narg('client_id')::bigint IS NULL OR m.role = 'admin' OR m.all_clients OR EXISTS (
        SELECT 1 FROM membership_clients mc
        WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id AND mc.client_id = sqlc.narg('client_id')::bigint))
ORDER BY lower(u.name), u.id;
