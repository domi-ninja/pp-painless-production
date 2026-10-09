#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/pp-context.sh"
s3_container="$(pp_container s3)"
app_access="$(node "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" STORAGE_ACCESS_KEY)"
app_secret="$(node "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" STORAGE_SECRET_KEY)"
[[ -n "$app_access" && -n "$app_secret" ]] || { echo 'Missing backend storage credentials' >&2; exit 1; }
buckets=()
for name in EXPORTS SNAPSHOT_IMPORTS MODULES FILES SEARCH; do
  buckets+=("$(node "$ROOT_DIR/scripts/env-file.cjs" get "$ENV_FILE" "S3_STORAGE_${name}_BUCKET")")
done
policy="$(node "$ROOT_DIR/scripts/storage-policy.cjs" "${buckets[@]}")"
remote_script='set -euo pipefail
container="$1"; shift
IFS= read -r app_access
IFS= read -r app_secret
IFS= read -r policy
network="$(docker inspect "$container" --format "{{.HostConfig.NetworkMode}}")"
root_user="$(docker inspect "$container" --format "{{range .Config.Env}}{{println .}}{{end}}" | sed -n "s/^MINIO_ROOT_USER=//p")"
root_secret="$(docker inspect "$container" --format "{{range .Config.Env}}{{println .}}{{end}}" | sed -n "s/^MINIO_ROOT_PASSWORD=//p")"
[[ -n "$network" && -n "$root_user" && -n "$root_secret" ]] || { echo "Missing MinIO configuration" >&2; exit 1; }
root_url="$(printf "%s\n%s\n" "$root_user" "$root_secret" | python3 -c '\''import sys,urllib.parse; u,p=[urllib.parse.quote(sys.stdin.readline().rstrip("\n"),safe="") for _ in range(2)]; print("http://"+u+":"+p+"@s3:9000")'\'')"
# All credentials enter the helper through stdin.
printf "%s\n%s\n%s\n%s\n" "$root_url" "$app_access" "$app_secret" "$policy" |
docker run --rm -i --network "$network" --entrypoint /bin/sh minio/mc:latest -eu -c '\''
  IFS= read -r MC_HOST_local; export MC_HOST_local
  IFS= read -r app_access
  IFS= read -r app_secret
  IFS= read -r policy
  policy_name="$1"; shift
  attempt=0
  until mc ready local >/dev/null 2>&1; do
    attempt=$((attempt + 1)); [ "$attempt" -lt 60 ] || { echo "MinIO is not ready" >&2; exit 1; }; sleep 1
  done
  for bucket in "$@"; do mc mb --ignore-existing "local/$bucket" >/dev/null; done
  printf "%s" "$policy" > /tmp/pp-storage-policy.json
  printf "%s\n%s\n" "$app_access" "$app_secret" | mc admin user add local >/dev/null
  mc admin policy create local "$policy_name" /tmp/pp-storage-policy.json >/dev/null
  mc admin policy attach local "$policy_name" --user "$app_access" >/dev/null
'\'' sh "$@"'
args=("$s3_container" "pp-$PP_PROJECT-$PP_ENVIRONMENT" "${buckets[@]}")
escaped=()
for arg in "${args[@]}"; do escaped+=("$(printf '%q' "$arg")"); done
printf '%s\n%s\n%s\n' "$app_access" "$app_secret" "$policy" |
  ssh -o BatchMode=yes -o ConnectTimeout=10 "$SSH_TARGET" "bash -c $(printf '%q' "$remote_script") -- ${escaped[*]}"
echo 'Configured backend storage buckets and account'
