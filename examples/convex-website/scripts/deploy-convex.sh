#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
cli="$ROOT_DIR/node_modules/.bin/convex"
[[ -x "$cli" ]] || { echo 'Install the app Convex dependency before deploying' >&2; exit 1; }
exec "$cli" deploy --env-file "$ENV_FILE"
