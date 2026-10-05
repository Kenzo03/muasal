#!/bin/sh
# run.sh [tickets]: seeds project LOAD (once) and runs the k6 load test
# against the stack on this machine (FSD §21.1). Run from deploy/.
set -eu
cd "$(dirname "$0")/.."
n=${1:-100000}
dc() { docker compose --env-file .env "$@"; }
if [ "$(dc exec -T db psql -U owner -d zettra -Atc "SELECT count(*) FROM projects WHERE key = 'LOAD'")" = 0 ]; then
  echo "Seeding $n tickets…"
  dc exec -T db psql -U owner -d zettra -v tickets="$n" -f - <loadtest/seed.sql >loadtest/tokens.txt
fi
url=$(grep '^PUBLIC_URL=' .env | cut -d= -f2)
docker run --rm --network host -v "$PWD/loadtest:/scripts:ro" -w /scripts grafana/k6:1.3.0 \
  run --quiet -e BASE="$url" -e TICKETS="$n" -e DURATION="${DURATION:-3m}" k6.js
