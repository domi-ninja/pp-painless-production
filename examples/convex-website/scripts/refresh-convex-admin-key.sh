#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
SERVICE="${PP_SERVICE:-convex-backend}"
[[ -f "$ENV_FILE" ]] || { echo "Missing selected environment file" >&2; exit 1; }

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

container="$(pp_container "$SERVICE")"

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
