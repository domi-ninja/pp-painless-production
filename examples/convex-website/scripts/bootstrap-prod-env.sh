#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SITE_HOST="${1:-}"
ENV_FILE="${2:-$ROOT_DIR/.env.local}"
BACKUP_FILE="$ENV_FILE.previous"

if [[ "$SITE_HOST" == --restore ]]; then
  [[ -f "$BACKUP_FILE" ]] || { echo "Error: no previous environment to restore" >&2; exit 1; }
  mv "$BACKUP_FILE" "$ENV_FILE"
  chmod 600 "$ENV_FILE"
  echo "Restored the previous local environment. Run pp deploy to apply it, including Convex function variables."
  exit 0
fi

if [[ -z "$SITE_HOST" ]]; then
  echo "Usage: scripts/bootstrap-prod-env.sh <site-hostname>|--restore [env-file]" >&2
  exit 1
fi

if [[ -e "$BACKUP_FILE" ]]; then
  echo "Error: an environment recovery copy already exists. Restore it with --restore, or archive it after verifying the previous rotation." >&2
  exit 1
fi

if [[ ! "$SITE_HOST" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ ]]; then
  echo "Error: invalid site hostname: $SITE_HOST" >&2
  exit 1
fi

PROJECT="$({
  awk '
    $1 == "project:" { in_project=1; next }
    in_project && $1 == "name:" {
      value=$2
      gsub(/^["'\'']|["'\'']$/, "", value)
      print value
      exit
    }
    in_project && $1 !~ /^ / && $1 != "" { in_project=0 }
  ' "$ROOT_DIR/deploy.yml"
})"

if [[ -z "$PROJECT" || ! "$PROJECT" =~ ^[a-z0-9][a-z0-9-]*$ ]]; then
  echo "Error: deploy.yml has no valid literal project.name" >&2
  exit 1
fi

API_HOST="api.$SITE_HOST"
DEFAULT_DB_NAME="${PROJECT//-/_}"

get_existing_nonsecret() {
  node - "$ENV_FILE" "$1" "$2" <<'NODE'
const fs = require('node:fs');
const [file, key, fallback] = process.argv.slice(2);
let value = '';
if (fs.existsSync(file)) {
  for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
    const separator = line.indexOf('=');
    if (separator < 0 || line.slice(0, separator).trim() !== key) continue;
    value = line.slice(separator + 1).trim();
    if (value.length >= 2 && ((value.startsWith('"') && value.endsWith('"')) ||
        (value.startsWith("'") && value.endsWith("'")))) value = value.slice(1, -1);
  }
}
process.stdout.write(value || fallback);
NODE
}

# Preserve names that point at existing data. Fresh copies without an env file
# receive names derived from the literal pp project slug.
INSTANCE_NAME="$(get_existing_nonsecret INSTANCE_NAME "$PROJECT")"
POSTGRES_DB="$(get_existing_nonsecret POSTGRES_DB "$DEFAULT_DB_NAME")"
POSTGRES_USER="$(get_existing_nonsecret POSTGRES_USER "$DEFAULT_DB_NAME")"
S3_STORAGE_EXPORTS_BUCKET="$(get_existing_nonsecret S3_STORAGE_EXPORTS_BUCKET "$PROJECT-convex-exports")"
S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET="$(get_existing_nonsecret S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET "$PROJECT-convex-snapshot-imports")"
S3_STORAGE_MODULES_BUCKET="$(get_existing_nonsecret S3_STORAGE_MODULES_BUCKET "$PROJECT-convex-modules")"
S3_STORAGE_FILES_BUCKET="$(get_existing_nonsecret S3_STORAGE_FILES_BUCKET "$PROJECT-convex-files")"
S3_STORAGE_SEARCH_BUCKET="$(get_existing_nonsecret S3_STORAGE_SEARCH_BUCKET "$PROJECT-convex-search")"

umask 077
ENV_DIR="$(dirname "$ENV_FILE")"
mkdir -p "$ENV_DIR"
TEMP_FILE="$(mktemp "$ENV_DIR/.env.local.XXXXXX")"
trap 'rm -f "$TEMP_FILE"' EXIT

INSTANCE_SECRET="$(openssl rand -hex 32)"
POSTGRES_PASSWORD="$(openssl rand -hex 32)"
POSTGRES_URL_USER="$(node -e 'process.stdout.write(encodeURIComponent(process.argv[1]))' -- "$POSTGRES_USER")"
MINIO_ROOT_USER="$(openssl rand -hex 12)"
MINIO_ROOT_PASSWORD="$(openssl rand -hex 32)"

{
  printf 'VITE_CONVEX_URL=https://%s\n' "$API_HOST"
  printf 'SITE_URL=https://%s\n' "$SITE_HOST"
  printf 'CONVEX_SELF_HOSTED_URL=https://%s\n' "$API_HOST"
  printf 'INSTANCE_NAME=%s\n' "$INSTANCE_NAME"
  printf 'INSTANCE_SECRET=%s\n' "$INSTANCE_SECRET"
  printf 'POSTGRES_DB=%s\n' "$POSTGRES_DB"
  printf 'POSTGRES_USER=%s\n' "$POSTGRES_USER"
  printf 'POSTGRES_PASSWORD=%s\n' "$POSTGRES_PASSWORD"
  printf 'POSTGRES_URL=postgres://%s:%s@postgres:5432?sslmode=disable\n' "$POSTGRES_URL_USER" "$POSTGRES_PASSWORD"
  printf 'MINIO_ROOT_USER=%s\n' "$MINIO_ROOT_USER"
  printf 'MINIO_ROOT_PASSWORD=%s\n' "$MINIO_ROOT_PASSWORD"
  printf 'S3_STORAGE_EXPORTS_BUCKET=%s\n' "$S3_STORAGE_EXPORTS_BUCKET"
  printf 'S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET=%s\n' "$S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET"
  printf 'S3_STORAGE_MODULES_BUCKET=%s\n' "$S3_STORAGE_MODULES_BUCKET"
  printf 'S3_STORAGE_FILES_BUCKET=%s\n' "$S3_STORAGE_FILES_BUCKET"
  printf 'S3_STORAGE_SEARCH_BUCKET=%s\n' "$S3_STORAGE_SEARCH_BUCKET"
  node "$ROOT_DIR/scripts/generate-jwt.js"
} > "$TEMP_FILE"

if [[ -f "$ENV_FILE" ]]; then
  cp -p "$ENV_FILE" "$BACKUP_FILE"
  chmod 600 "$BACKUP_FILE"
fi

chmod 600 "$TEMP_FILE"
mv "$TEMP_FILE" "$ENV_FILE"
trap - EXIT

echo "Rotated production credentials in $ENV_FILE"
echo "No running services were changed. Deploy to apply; use --restore and deploy to recover."
echo "Preserved existing data-bearing names when present."
echo "The Convex admin key will be generated from the healthy backend during deploy."
