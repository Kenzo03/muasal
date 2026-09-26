-- Git links (FSD §14.1).

-- name: CreateRepo :one
INSERT INTO git_repos (project_id, provider, name, web_url, secret_enc)
VALUES (sqlc.arg('project_id'), sqlc.arg('provider'), sqlc.arg('name'), sqlc.arg('web_url'), sqlc.arg('secret_enc'))
RETURNING *;

-- name: ListRepos :many
SELECT * FROM git_repos WHERE project_id = $1 ORDER BY lower(name), id;

-- name: GetRepo :one
SELECT * FROM git_repos WHERE id = $1;

-- name: UpdateRepo :one
UPDATE git_repos SET name = sqlc.arg('name'), web_url = sqlc.arg('web_url'),
  secret_enc = coalesce(sqlc.narg('secret_enc'), secret_enc)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteRepo :exec
DELETE FROM git_repos WHERE id = $1;

-- name: InsertDelivery :one
INSERT INTO webhook_deliveries (repo_id, event, payload) VALUES ($1, $2, $3) RETURNING id;

-- name: GetDelivery :one
SELECT d.*, r.provider FROM webhook_deliveries d JOIN git_repos r ON r.id = d.repo_id WHERE d.id = $1;

-- name: DeleteDelivery :exec
DELETE FROM webhook_deliveries WHERE id = $1;

-- name: UpsertCommit :one
-- Re-deliveries are idempotent on repository plus SHA (§14.1).
INSERT INTO commits (repo_id, sha, message, author_name, author_email, committed_at, url)
VALUES (sqlc.arg('repo_id'), sqlc.arg('sha'), sqlc.arg('message'), sqlc.narg('author_name'), sqlc.narg('author_email'),
        sqlc.narg('committed_at'), sqlc.narg('url'))
ON CONFLICT (repo_id, sha) DO UPDATE SET message = excluded.message
RETURNING id;

-- name: UpsertMergeRequest :one
INSERT INTO merge_requests (repo_id, number, title, state, merged_at, url)
VALUES (sqlc.arg('repo_id'), sqlc.arg('number'), sqlc.arg('title'), sqlc.arg('state'), sqlc.narg('merged_at'), sqlc.narg('url'))
ON CONFLICT (repo_id, number) DO UPDATE SET title = excluded.title, state = excluded.state,
  merged_at = coalesce(excluded.merged_at, merge_requests.merged_at), url = coalesce(excluded.url, merge_requests.url)
RETURNING id;

-- name: LinkCommit :exec
INSERT INTO ticket_commits (ticket_id, commit_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: LinkMergeRequest :exec
INSERT INTO ticket_merge_requests (ticket_id, mr_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: TicketIDsByKeys :many
-- Keys that name an existing ticket; a key counts only when the project and the ticket exist (§14.1).
SELECT id FROM tickets WHERE key = ANY (sqlc.arg('keys')::text[]);

-- name: ListTicketCommits :many
SELECT c.*, r.name AS repo_name FROM ticket_commits tc
JOIN commits c ON c.id = tc.commit_id JOIN git_repos r ON r.id = c.repo_id
WHERE tc.ticket_id = $1 ORDER BY c.committed_at DESC NULLS LAST, c.id DESC;

-- name: ListTicketMergeRequests :many
SELECT m.*, r.name AS repo_name FROM ticket_merge_requests tm
JOIN merge_requests m ON m.id = tm.mr_id JOIN git_repos r ON r.id = m.repo_id
WHERE tm.ticket_id = $1 ORDER BY m.id DESC;

-- name: UserIDsByEmails :many
SELECT id FROM users WHERE lower(email) = ANY (sqlc.arg('emails')::text[]);
