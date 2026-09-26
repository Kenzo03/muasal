#!/bin/sh
# upgrade.sh <bundle.tar> (FSD §19.3): takes a backup, loads the new images and
# restarts; the app applies forward-only migrations as it starts. To roll back,
# restore the pre-upgrade backup with ./restore.sh and start the old images.
set -eu
cd "$(dirname "$0")"
bundle=${1:?usage: ./upgrade.sh muasal-<version>.tar}
dc() { docker compose --env-file .env "$@"; }
before=$(dc exec -T backup sh -c 'ls /backups/db-*.dump 2>/dev/null | wc -l')
echo "Taking a backup first…"
dc exec -T backup touch /backups/requests/run-now
i=0
until [ "$(dc exec -T backup sh -c 'ls /backups/db-*.dump 2>/dev/null | wc -l')" -gt "$before" ]; do
  i=$((i + 1)); [ $i -gt 60 ] && { echo "upgrade: the backup did not finish in 10 minutes" >&2; exit 1; }
  sleep 10
done
echo "Loading $bundle…"
tmp=$(mktemp -d)
tar -xf "$bundle" -C "$tmp"
new=$(find "$tmp" -maxdepth 2 -name VERSION | head -1)
dir=$(dirname "$new")
docker load -i "$dir/images.tar"
# The new release's deploy files, keeping .env, certificates and the TLS snippet.
for f in compose.yaml compose.host-ai.yaml Caddyfile backup.sh restore.sh install.sh upgrade.sh VERSION; do
  [ -f "$dir/$f" ] && cp "$dir/$f" .
done
version=$(cat VERSION)
sed -i.bak "s/^VERSION=.*/VERSION=$version/" .env && rm -f .env.bak
rm -rf "$tmp"
profile=""
dc ps --services | grep -qx model && profile="--profile local-ai"
# shellcheck disable=SC2086
dc $profile up -d --wait --pull never
echo "Upgraded to $version."
