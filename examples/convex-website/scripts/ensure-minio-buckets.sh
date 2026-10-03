#!/usr/bin/env bash

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.local}"
SSH_TARGET="${PP_HOST:-$(awk '$1 == "ssh:" { print $2; exit }' deploy.yml)}"
PROJECT="${PP_PROJECT:-}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: $ENV_FILE file not found" >&2
  exit 1
fi

if [[ -z "$PROJECT" ]]; then
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
    ' deploy.yml
  })"
fi

if [[ -z "$PROJECT" ]]; then
  echo "Error: could not determine project name from PP_PROJECT or deploy.yml" >&2
  exit 1
fi

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
project="$1"
shift
s3_container="$(docker ps --filter "label=pp.project=$project" --filter "label=pp.service=s3" --format "{{.Names}}" | head -n 1)"
if [ -z "$s3_container" ]; then
  echo "Error: no running s3 container found for pp.project=$project" >&2
  exit 1
fi
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

remote_args=("$PROJECT" "${buckets[@]}")
escaped_args=()
for value in "${remote_args[@]}"; do
  escaped_args+=("$(printf '%q' "$value")")
done

ssh "$SSH_TARGET" "bash -c $(printf '%q' "$remote_script") -- ${escaped_args[*]}"

echo "Ensured MinIO buckets exist on $SSH_TARGET"
