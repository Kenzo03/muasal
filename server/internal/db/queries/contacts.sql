-- name: ListContacts :many
-- R-AC-6: a contact is visible when its client is in the user's scope in some
-- project; internal contacts (no client) are visible to every project member.
SELECT c.id, c.client_id, cl.name AS client_name, c.name, c.title, c.email, c.phone
FROM contacts c
LEFT JOIN clients cl ON cl.id = c.client_id
WHERE (sqlc.arg('is_admin')::boolean
       OR (c.client_id IS NULL AND EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = sqlc.arg('user_id')))
       OR c.client_id IN (SELECT pc.client_id
                          FROM memberships m
                          JOIN project_clients pc ON pc.project_id = m.project_id
                          WHERE m.user_id = sqlc.arg('user_id') AND m.all_clients
                          UNION ALL
                          SELECT mc.client_id FROM membership_clients mc WHERE mc.user_id = sqlc.arg('user_id')))
  AND (sqlc.narg('id')::bigint IS NULL OR c.id = sqlc.narg('id')::bigint)
  AND (sqlc.narg('client_id')::bigint IS NULL OR c.client_id = sqlc.narg('client_id')::bigint)
  AND c.name ILIKE '%' || sqlc.arg('q')::text || '%'
ORDER BY lower(c.name), c.id
LIMIT 50;

-- name: CanEditContactsOf :one
-- Members and project admins keep the contacts of clients in their scope, and
-- internal contacts (client NULL) in any project they belong to.
SELECT EXISTS (
  SELECT 1 FROM memberships m
  WHERE m.user_id = sqlc.arg('user_id') AND m.role IN ('admin', 'member')
    AND (sqlc.narg('client_id')::bigint IS NULL
         OR (m.all_clients AND EXISTS (SELECT 1 FROM project_clients pc
                                       WHERE pc.project_id = m.project_id
                                         AND pc.client_id = sqlc.narg('client_id')::bigint))
         OR EXISTS (SELECT 1 FROM membership_clients mc
                    WHERE mc.user_id = m.user_id AND mc.project_id = m.project_id
                      AND mc.client_id = sqlc.narg('client_id')::bigint))
);

-- name: CreateContact :one
INSERT INTO contacts (client_id, name, title, email, phone)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: UpdateContact :exec
UPDATE contacts SET client_id = $2, name = $3, title = $4, email = $5, phone = $6
WHERE id = $1;
