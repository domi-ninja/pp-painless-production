#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
SERVICE="${PP_SERVICE:-convex-backend}"
[[ -f "$ENV_FILE" ]] || { echo "Missing selected environment file" >&2; exit 1; }


container="$(pp_container "$SERVICE")"

admin_key="$(
  ssh -o BatchMode=yes -o ConnectTimeout=10 "$SSH_TARGET" "docker exec $(printf '%q' "$container") /convex/generate_admin_key.sh | tail -n 1"
)"

if [[ -z "$admin_key" || "$admin_key" != *"|"* ]]; then
  echo "Error: Convex container returned an invalid admin key" >&2
  exit 1
fi

instance_name="${INSTANCE_NAME:-$(node "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" INSTANCE_NAME)}"
key_instance="${admin_key%%|*}"
if [[ -n "$instance_name" && "$key_instance" != "$instance_name" ]]; then
  echo "Error: generated admin key is for $key_instance, expected $instance_name" >&2
  exit 1
fi

printf '%s' "$admin_key" | node "$ROOT_DIR/scripts/env-file.cjs" set "$ENV_FILE" CONVEX_SELF_HOSTED_ADMIN_KEY

echo "Updated CONVEX_SELF_HOSTED_ADMIN_KEY in $ENV_FILE from $container"
