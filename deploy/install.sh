#!/bin/sh
# install.sh (FSD §19.3): installs Muasal from this folder, the unpacked
# offline bundle. It loads the images, writes .env with fresh secrets, copies
# the model files for local AI, starts the stack, creates the first admin and
# prints their setup link. After that the server needs no internet in local
# or off AI mode.
#
#   ./install.sh                      asks for the host, AI mode, tier and admin
#   ./install.sh --yes --url https://muasal.example.co.id --ai local --tier recommended \
#                --admin-email it@example.co.id --admin-name "IT Admin"
#   ./install.sh --online ...         pulls images and models instead of the bundle
set -eu
cd "$(dirname "$0")"

url="" ai="" tier="" email="" name="" cert="" key="" yes=false online=false
while [ $# -gt 0 ]; do
  case "$1" in
    --url) url=$2; shift 2 ;;
    --ai) ai=$2; shift 2 ;;
    --tier) tier=$2; shift 2 ;;
    --admin-email) email=$2; shift 2 ;;
    --admin-name) name=$2; shift 2 ;;
    --cert) cert=$2; shift 2 ;;
    --key) key=$2; shift 2 ;;
    --yes) yes=true; shift ;;
    --online) online=true; shift ;;
    -h | --help) sed -n '2,15p' "$0"; exit 0 ;;
    *) echo "install: unknown option $1" >&2; exit 2 ;;
  esac
done

ask() { # ask VAR "question" default
  eval "cur=\${$1}"
  [ -n "$cur" ] && return
  if $yes; then eval "$1=\$3"; return; fi
  printf '%s [%s]: ' "$2" "$3"
  read -r answer
  eval "$1=\${answer:-\$3}"
}
fail() { echo "install: $*" >&2; exit 1; }

command -v docker >/dev/null || fail "Docker Engine is required"
docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required"

ask url "Public URL of this server" "http://$(hostname -f 2>/dev/null || hostname)"
ask ai "AI mode: off, local or byok" "local"
case "$ai" in off | local | byok) ;; *) fail "--ai must be off, local or byok" ;; esac
[ "$ai" = local ] && ask tier "Hardware tier: minimum or recommended (GPU)" "minimum"
cert="" key=""
case "$url" in https://*)
  ask cert "TLS certificate file (PEM), or 'internal' for Caddy's own CA" "internal"
  [ "$cert" = internal ] || ask key "TLS private key file (PEM)" "" ;;
esac
ask email "First admin's email" "admin@example.com"
ask name "First admin's name" "Admin"

version=$(cat VERSION 2>/dev/null || echo dev)
if $online; then
  echo "Pulling images for $version…"
  VERSION=$version docker compose --env-file .env.example pull --ignore-buildable
else
  [ -f images.tar ] || fail "images.tar is missing; run from the unpacked bundle, or use --online"
  echo "Loading images…"
  docker load -i images.tar
fi

if [ ! -f .env ]; then
  secret() { head -c "$1" /dev/urandom | base64 | tr -d '\n/+=' | head -c "$2"; }
  port=$(echo "$url" | sed -n 's#^[a-z]*://[^/:]*:\([0-9]*\).*#\1#p')
  [ -n "$port" ] || { case "$url" in https://*) port=443 ;; *) port=80 ;; esac; }
  cat >.env <<EOF
# Written by install.sh on $(date -u +%Y-%m-%dT%H:%MZ). Keep it private: it holds the database passwords.
VERSION=$version
PUBLIC_URL=$url
HTTP_PORT=$port
DB_OWNER_PASSWORD=$(secret 48 32)
DB_APP_PASSWORD=$(secret 48 32)
APP_SECRET_KEY=$(head -c 32 /dev/urandom | base64)
TZ=${TZ:-Asia/Jakarta}
ASK_LOG_RETENTION_DAYS=365
EOF
  chmod 600 .env
  echo "Wrote .env with new secrets."
else
  echo "Keeping the existing .env."
fi

mkdir -p caddy.d certs
case "$url" in
  https://*)
    if [ "$cert" = internal ]; then
      echo "tls internal" >caddy.d/tls.caddy
    else
      cp "$cert" certs/cert.pem && cp "$key" certs/key.pem && chmod 600 certs/key.pem
      echo "tls /certs/cert.pem /certs/key.pem" >caddy.d/tls.caddy
    fi ;;
  *) rm -f caddy.d/tls.caddy ;;
esac

profile=""
if [ "$ai" = local ]; then
  profile="--profile local-ai"
  if [ -d models ] && ! $online; then
    echo "Copying the model files into the models volume…"
    docker volume create muasal_models >/dev/null
    docker run --rm --entrypoint /bin/sh -v muasal_models:/root/.ollama -v "$PWD/models:/src:ro" \
      "$(grep -o 'ollama/ollama:[^ ]*' compose.yaml | head -1)" -c 'cp -a /src/. /root/.ollama/'
  fi
fi

echo "Starting Muasal…"
# shellcheck disable=SC2086
docker compose --env-file .env $profile up -d --wait --pull never

if [ "$ai" = local ] && $online; then
  for m in "$([ "$tier" = recommended ] && echo qwen3.5:9b || echo qwen3.5:4b)" bge-m3; do
    docker compose --env-file .env exec -T model ollama pull "$m"
  done
fi

link=$(docker compose --env-file .env exec -T app /app admin create-admin --email "$email" --name "$name")
if [ "$ai" = local ]; then
  docker compose --env-file .env exec -T app /app admin ai-local --url http://model:11434/v1 --tier "$tier"
fi
echo
echo "$link"
[ "$ai" = byok ] && echo "Then open Admin → AI to enter your provider's URL and key."
echo "Muasal is running at $url."
