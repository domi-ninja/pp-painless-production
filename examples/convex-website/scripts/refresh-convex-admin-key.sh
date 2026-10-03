#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.local}"
SSH_TARGET="${PP_HOST:-$(awk '$1 == "ssh:" { print $2; exit }' deploy.yml)}"
PROJECT="${PP_PROJECT:-}"
SERVICE="${PP_SERVICE:-convex-backend}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: $ENV_FILE file not found. Run scripts/bootstrap-prod-env.sh first." >&2
  exit 1
fi

if [[ -z "$PROJECT" ]]; then
  PROJECT="$(
    awk '
      $1 == "project:" { in_project=1; next }
      in_project && $1 == "name:" {
        value=$2
        gsub(/^["'\'']|["'\'']$/, "", value)
        print value
        exit
      }
      in_project && $1 !~ /^ / && $1 != "" { in_project=0 }
    ' deploy.yml
  )"
fi

if [[ -z "$PROJECT" ]]; then
  echo "Error: could not determine project name from PP_PROJECT or deploy.yml" >&2
  exit 1
fi

get_env() {
  local key="$1"
  local line value
  line="$(grep -E "^${key}=" "$ENV_FILE" | tail -n 1 || true)"
  value="${line#*=}"
  if [[ "$value" =~ ^\"(.*)\"$ ]]; then
    value="${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
    value="${BASH_REMATCH[1]}"
  fi
  printf '%s' "$value"
}

container="$(
  ssh "$SSH_TARGET" \
    "docker ps --filter label=pp.project=$PROJECT --filter label=pp.service=$SERVICE --format '{{.Names}}' | head -n 1"
)"

if [[ -z "$container" ]]; then
  echo "Error: no running $SERVICE container found for pp.project=$PROJECT on $SSH_TARGET" >&2
  exit 1
fi

admin_key="$(
  ssh "$SSH_TARGET" "docker exec $(printf '%q' "$container") /convex/generate_admin_key.sh | tail -n 1"
)"

if [[ -z "$admin_key" || "$admin_key" != *"|"* ]]; then
  echo "Error: Convex container returned an invalid admin key" >&2
  exit 1
fi

instance_name="${INSTANCE_NAME:-$(get_env INSTANCE_NAME)}"
key_instance="${admin_key%%|*}"
if [[ -n "$instance_name" && "$key_instance" != "$instance_name" ]]; then
  echo "Error: generated admin key is for $key_instance, expected $instance_name" >&2
  exit 1
fi

tmp="$(mktemp)"
CONVEX_ADMIN_KEY="$admin_key" awk '
  BEGIN { done=0; key=ENVIRON["CONVEX_ADMIN_KEY"] }
  /^CONVEX_SELF_HOSTED_ADMIN_KEY=/ {
    if (!done) {
      print "CONVEX_SELF_HOSTED_ADMIN_KEY=" key
      done=1
    }
    next
  }
  { print }
  END {
    if (!done) print "CONVEX_SELF_HOSTED_ADMIN_KEY=" key
  }
' "$ENV_FILE" > "$tmp"
mv "$tmp" "$ENV_FILE"
chmod 600 "$ENV_FILE"

echo "Updated CONVEX_SELF_HOSTED_ADMIN_KEY in $ENV_FILE from $container"
