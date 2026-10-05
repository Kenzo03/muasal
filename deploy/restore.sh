#!/bin/sh
# restore.sh <dump> (FSD §19.4): puts a backup back. It stops the app and the
# web server, restores the database and the attachments, starts them again and
# queues a re-index, since the dump leaves out the vectors.
#   ./restore.sh db-20260926-0100.dump        a file in the backups volume
set -eu
cd "$(dirname "$0")"
dump=${1:?usage: ./restore.sh db-YYYYMMDD-HHMM.dump}
dc() { docker compose --env-file .env "$@"; }
dc exec -T backup test -f "/backups/$(basename "$dump")" || { echo "restore: /backups/$(basename "$dump") not found" >&2; exit 1; }
echo "Stopping app and web…"
dc stop app web
echo "Restoring the database…"
dc exec -T backup pg_restore -h db -U owner -d zettra --clean --if-exists --no-owner --role=owner "/backups/$(basename "$dump")"
echo "Restoring attachments…"
dc run --rm --no-deps --entrypoint sh -v zettra_attachments:/data/attachments backup -c 'cp -Rn /backups/attachments/. /data/attachments/'
echo "Starting app and web…"
dc up -d --wait app web
dc exec -T app /app admin reindex --all
echo "Restored $(basename "$dump")."
