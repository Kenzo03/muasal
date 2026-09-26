-- Load-test data (FSD §18, §21.1): project LOAD with 100,000 tickets, two
-- comments each and ten search chunks per ticket (1,000,000), 300 menus,
-- 5 clients, 200 contacts and 50 members. Chunks carry no vectors: the load
-- test runs Ask on the keyword path. It prints one session token per member,
-- for k6, since sign-in allows 20 attempts per IP per minute.
--   psql -U owner -d muasal -v tickets=100000 -f seed.sql > tokens.txt
\set ON_ERROR_STOP on
\set QUIET on
\if :{?tickets}
\else
  \set tickets 100000
\endif
BEGIN;
INSERT INTO projects (key, name) VALUES ('LOAD', 'Load test') RETURNING id AS pid \gset
CREATE TEMP TABLE words (i int, w text);
INSERT INTO words SELECT row_number() OVER (), w FROM unnest(string_to_array(
  'overtime approval supervisor payroll leave balance attendance shift schedule cutoff bonus tax allowance ' ||
  'lembur persetujuan atasan gaji cuti saldo absensi jadwal potongan tunjangan pajak laporan export import ' ||
  'report dashboard mobile reminder holiday contract probation resign transfer promotion rounding late early ' ||
  'klaim reimbursement medical insurance loan kasbon approval flow level manager director branch region', ' ')) AS w;
CREATE TEMP TABLE nw AS SELECT count(*)::int AS n FROM words;

INSERT INTO clients (name, code) SELECT 'Load Client ' || i, 'LC' || i FROM generate_series(1, 5) i;
INSERT INTO project_clients SELECT :pid, id FROM clients WHERE name LIKE 'Load Client %';
INSERT INTO contacts (client_id, name, title)
SELECT (SELECT id FROM clients WHERE name = 'Load Client ' || (1 + i % 5)), 'Contact ' || i, 'HR Staff'
FROM generate_series(1, 200) i;
INSERT INTO users (email, name, password_hash)
SELECT 'load' || i || '@example.com', 'Load User ' || i, NULL FROM generate_series(1, 50) i;
INSERT INTO memberships (user_id, project_id, role, all_clients)
SELECT id, :pid, 'member', true FROM users WHERE email LIKE 'load%@example.com';

INSERT INTO nodes (project_id, type, name, position)
SELECT :pid, 'module', 'Module ' || i, i FROM generate_series(1, 20) i;
INSERT INTO nodes (project_id, parent_id, type, name, position)
SELECT :pid, m.id, 'menu', m.name || ' Menu ' || j, j
FROM nodes m, generate_series(1, 15) j WHERE m.project_id = :pid AND m.type = 'module';

CREATE TEMP TABLE ids AS SELECT
  (SELECT array_agg(id ORDER BY id) FROM statuses WHERE project_id = :pid) AS statuses,
  (SELECT array_agg(id ORDER BY id) FROM statuses WHERE project_id = :pid AND category IN ('done', 'cancelled')) AS closed,
  (SELECT array_agg(client_id ORDER BY client_id) FROM project_clients WHERE project_id = :pid) AS clients,
  (SELECT array_agg(id ORDER BY id) FROM contacts WHERE name LIKE 'Contact %') AS contacts,
  (SELECT array_agg(id ORDER BY id) FROM users WHERE email LIKE 'load%@example.com') AS users,
  (SELECT array_agg(id ORDER BY id) FROM nodes WHERE project_id = :pid AND type = 'menu') AS menus;

-- phrase(seed, n): n vocabulary words picked by seed.
CREATE FUNCTION pg_temp.phrase(seed bigint, n int) RETURNS text LANGUAGE sql STABLE AS $$
  SELECT string_agg(w, ' ' ORDER BY k) FROM generate_series(1, n) k
  JOIN words ON words.i = 1 + ((seed * 7919 + k * 104729) % (SELECT n FROM nw))
$$;

INSERT INTO tickets (project_id, number, key, type, title, description, reason, status_id, client_id,
                     requester_contact_id, reporter_id, assignee_id, priority, created_at, updated_at, closed_at)
SELECT :pid, g, 'LOAD-' || g, (ARRAY['bug', 'change_request', 'feature'])[1 + g % 3],
       initcap(pg_temp.phrase(g, 5)) || ' ' || g, pg_temp.phrase(g + 1, 30), pg_temp.phrase(g + 2, 12),
       s.status, CASE WHEN g % 5 = 0 THEN NULL ELSE ids.clients[1 + g % 5] END,
       ids.contacts[1 + g % 200], ids.users[1 + g % 50], ids.users[1 + (g * 7) % 50],
       (ARRAY['low', 'medium', 'high', 'urgent'])[1 + g % 4], c.at, c.at,
       CASE WHEN s.status = ANY (ids.closed) THEN c.at + interval '5 days' END
FROM generate_series(1, :tickets) g, ids,
     LATERAL (SELECT ids.statuses[1 + (g * 13) % 5] AS status) s,
     LATERAL (SELECT now() - (g % 1095) * interval '1 day' AS at) c;
UPDATE projects SET ticket_seq = :tickets WHERE id = :pid;

INSERT INTO ticket_nodes (ticket_id, node_id)
SELECT t.id, ids.menus[1 + t.number % 300] FROM tickets t, ids WHERE t.project_id = :pid
UNION
SELECT t.id, ids.menus[1 + (t.number * 31) % 300] FROM tickets t, ids WHERE t.project_id = :pid AND t.number % 3 = 0;

INSERT INTO comments (ticket_id, author_id, internal, body, created_at)
SELECT t.id, ids.users[1 + (t.number + k) % 50], k = 1, pg_temp.phrase(t.number * 3 + k, 25), t.created_at + k * interval '1 hour'
FROM tickets t, ids, generate_series(1, 2) k WHERE t.project_id = :pid;

INSERT INTO chunks (source_type, source_id, seq, ticket_id, project_id, client_id, node_ids, user_ids, contact_ids,
                    occurred_at, content, content_hash)
SELECT 'ticket', t.id, k, t.id, t.project_id, t.client_id, ARRAY[ids.menus[1 + t.number % 300]],
       ARRAY[t.reporter_id], ARRAY[t.requester_contact_id], coalesce(t.closed_at, t.created_at),
       '[' || t.key || '] ' || pg_temp.phrase(t.number * 10 + k, 60), sha256(convert_to(t.key || ':' || k, 'UTF8'))
FROM tickets t, ids, generate_series(0, 9) k WHERE t.project_id = :pid;

CREATE TEMP TABLE tokens AS
SELECT u.id, md5(random()::text) || md5(random()::text) AS token FROM users u WHERE u.email LIKE 'load%@example.com';
INSERT INTO sessions (token_hash, user_id, expires_at)
SELECT sha256(convert_to(token, 'UTF8')), id, now() + interval '6 days' FROM tokens;
COMMIT;
ANALYZE;
\pset tuples_only on
\pset format unaligned
SELECT token FROM tokens ORDER BY id;
