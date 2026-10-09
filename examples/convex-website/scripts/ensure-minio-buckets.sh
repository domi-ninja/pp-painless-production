#!/usr/bin/env bash

set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
[[ -f "$ENV_FILE" ]] || { echo "Missing selected environment file" >&2; exit 1; }
s3_container="$(pp_container s3)"

bucket_keys=(
  S3_STORAGE_EXPORTS_BUCKET
  S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET
  S3_STORAGE_MODULES_BUCKET
  S3_STORAGE_FILES_BUCKET
  S3_STORAGE_SEARCH_BUCKET
)

buckets=()
for name in "${bucket_keys[@]}"; do
  value="$(sed -n "s/^${name}=//p" "$ENV_FILE" | tail -n 1)"
  if [[ -z "$value" ]]; then
    echo "Error: $name is missing from $ENV_FILE" >&2
    exit 1
  fi
  buckets+=("$value")
done

remote_script='set -euo pipefail
s3_container="$1"
shift
network="$(docker inspect "$s3_container" --format "{{.HostConfig.NetworkMode}}")"
access_key="$(docker inspect "$s3_container" --format "{{range .Config.Env}}{{println .}}{{end}}" | sed -n "s/^MINIO_ROOT_USER=//p" | tail -n 1)"
secret_key="$(docker inspect "$s3_container" --format "{{range .Config.Env}}{{println .}}{{end}}" | sed -n "s/^MINIO_ROOT_PASSWORD=//p" | tail -n 1)"
if [ -z "$network" ] || [ -z "$access_key" ] || [ -z "$secret_key" ]; then
  echo "Error: incomplete MinIO container configuration" >&2
  exit 1
fi
# Credentials enter the helper through stdin and stay out of its argv/config.
printf "%s\n%s\n" "$access_key" "$secret_key" |
docker run --rm -i --network "$network" --entrypoint /bin/sh minio/mc:latest -eu -c '\''
  IFS= read -r access_key
  IFS= read -r secret_key
  export MC_HOST_local="http://$access_key:$secret_key@s3:9000"
  for bucket in "$@"; do
    mc mb --ignore-existing "local/$bucket" >/dev/null
  done
'\'' sh "$@"'

remote_args=("$s3_container" "${buckets[@]}")
escaped_args=()
for value in "${remote_args[@]}"; do
  escaped_args+=("$(printf '%q' "$value")")
done

ssh "$SSH_TARGET" "bash -c $(printf '%q' "$remote_script") -- ${escaped_args[*]}"

echo "Ensured MinIO buckets exist on $SSH_TARGET"
