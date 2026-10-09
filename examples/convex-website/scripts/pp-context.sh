#!/usr/bin/env bash

# Hooks receive their selected deployment from pp. Manual runs need the same values.
: "${PP_PROJECT:?Set PP_PROJECT}"
: "${PP_ENVIRONMENT:?Set PP_ENVIRONMENT}"
: "${PP_HOST:?Set PP_HOST to the selected SSH target}"
: "${PP_ENV_FILE:?Set PP_ENV_FILE to the selected environment file}"
for value in "$PP_PROJECT" "$PP_ENVIRONMENT"; do
  [[ "$value" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || { echo "Invalid deployment identity" >&2; exit 1; }
done
[[ "$PP_HOST" =~ ^([a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$ ]] || { echo "Invalid SSH target" >&2; exit 1; }
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$PP_ENV_FILE"
[[ "$ENV_FILE" = /* ]] || ENV_FILE="$ROOT_DIR/$ENV_FILE"
SSH_TARGET="$PP_HOST"
PROJECT="$PP_PROJECT"

pp_container() {
  local service="$1" result
  [[ "$service" =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?$ ]] || return 1
  result="$(ssh -o BatchMode=yes -o ConnectTimeout=10 "$SSH_TARGET" "docker ps --filter label=pp.project=$PP_PROJECT --filter label=pp.environment=$PP_ENVIRONMENT --filter label=pp.service=$service --format '{{.Names}}'")" || return
  [[ -n "$result" && "$result" != *$'\n'* ]] || { echo "Expected exactly one $service container for $PP_PROJECT/$PP_ENVIRONMENT" >&2; return 1; }
  printf '%s' "$result"
}
