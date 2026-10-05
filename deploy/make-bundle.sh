#!/bin/sh
# make-bundle.sh <version> [--build] [--models DIR] (FSD §19): writes
# dist/zettra-<version>.tar, the offline bundle: every image the stack runs,
# the deploy files and, with --models, an Ollama models folder (the
# ~/.ollama/models of a machine that pulled qwen3.5 and bge-m3).
# --build builds zettra-app and zettra-web first; otherwise they must exist.
set -eu
cd "$(dirname "$0")/.."
version=${1:?usage: deploy/make-bundle.sh <version> [--build] [--models DIR]}
shift
build=false models=""
while [ $# -gt 0 ]; do
  case "$1" in
    --build) build=true; shift ;;
    --models) models=$2; shift 2 ;;
    *) echo "make-bundle: unknown option $1" >&2; exit 2 ;;
  esac
done
if $build; then
  docker build -t "zettra-app:$version" server
  docker build -t "zettra-web:$version" web
fi
images="zettra-app:$version zettra-web:$version"
for img in $(grep -oE 'image: [^$ ]+$' deploy/compose.yaml | awk '{print $2}' | sort -u); do
  case "$img" in ollama/*) [ -n "$models" ] && images="$images $img" ;; *) images="$images $img" ;; esac
done
# The stack's other images (Caddy, PostgreSQL) come from Docker Hub. Pull any
# that is missing, for the bundle's platform (amd64, as docs/operations.md
# says), so a clean machine such as the release runner can save them.
for img in $images; do
  case "$img" in zettra-*) continue ;; esac
  docker image inspect "$img" >/dev/null 2>&1 || docker pull --platform linux/amd64 "$img"
done
out=dist/zettra-$version
rm -rf "$out" && mkdir -p "$out/caddy.d" "$out/certs"
echo "Saving images: $images"
# shellcheck disable=SC2086
docker save -o "$out/images.tar" $images
grep -v '^    build: ' deploy/compose.yaml >"$out/compose.yaml" # the bundle has images, not sources
cp deploy/compose.host-ai.yaml deploy/Caddyfile deploy/.env.example deploy/install.sh deploy/upgrade.sh \
  deploy/backup.sh deploy/restore.sh "$out/"
cp docs/operations.md "$out/README.md"
echo "$version" >"$out/VERSION"
if [ -n "$models" ]; then
  mkdir -p "$out/models" && cp -a "$models" "$out/models/models"
fi
tar -C dist -cf "dist/zettra-$version.tar" "zettra-$version"
rm -rf "$out"
echo "Wrote dist/zettra-$version.tar ($(du -h "dist/zettra-$version.tar" | cut -f1))."
