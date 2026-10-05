#!/bin/sh
# backup.sh (FSD §19.4): the backup service's loop. Daily at 01:00 server time,
# or within 30 seconds of "Run backup now", it dumps the database without the
# rebuildable vectors, copies new attachments and keeps 14 days of dumps.
set -eu
mkdir -p /backups/requests /backups/attachments
chmod 0777 /backups/requests # the app, a non-root user, leaves run-now here
last=""
while true; do
  now=$(date +%H%M)
  if { [ "$now" = "0100" ] && [ "$last" != "$(date +%Y%m%d)" ]; } || [ -f /backups/requests/run-now ]; then
    rm -f /backups/requests/run-now
    ts=$(date +%Y%m%d-%H%M)
    skip="--exclude-table-data=chunks" # vectors are rebuildable
    [ "${BACKUP_INCLUDE_VECTORS:-false}" = "true" ] && skip=""
    if pg_dump -h db -U owner -d zettra -Fc $skip -f "/backups/db-$ts.dump.tmp"; then
      mv "/backups/db-$ts.dump.tmp" "/backups/db-$ts.dump"
      cp -Rn /data/attachments/. /backups/attachments/ # files are immutable, named by hash
      find /backups -maxdepth 1 -name 'db-*.dump' -mtime +14 -delete
      echo "backup: /backups/db-$ts.dump"
    else
      rm -f "/backups/db-$ts.dump.tmp"
      echo "backup: pg_dump failed" >&2
    fi
    last=$(date +%Y%m%d)
  fi
  sleep 30
done
