#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
cli="$ROOT_DIR/node_modules/convex/bin/main.js"
[[ -f "$cli" ]] || { echo 'Install the app Convex dependency before deploying' >&2; exit 1; }
export CONVEX_SELF_HOSTED_URL="$(node -- "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" CONVEX_SELF_HOSTED_URL)"
export CONVEX_SELF_HOSTED_ADMIN_KEY="$(node -- "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" CONVEX_SELF_HOSTED_ADMIN_KEY)"
[[ -n "$CONVEX_SELF_HOSTED_URL" && -n "$CONVEX_SELF_HOSTED_ADMIN_KEY" ]] || { echo 'Selected Convex URL or admin key is missing' >&2; exit 1; }
for name in SITE_URL JWT_PRIVATE_KEY JWKS; do
  value="$(node -- "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" "$name")"
  [[ -n "$value" ]] || { echo "Missing $name" >&2; exit 1; }
  if ! printf '%s' "$value" | node -- "$cli" env set --env-file "$ENV_FILE" -- "$name" >/dev/null 2>&1; then
    echo "Could not set $name in the selected Convex deployment" >&2
    exit 1
  fi
  echo "Set $name"
done
