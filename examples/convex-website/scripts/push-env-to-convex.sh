#!/usr/bin/env bash

set -euo pipefail

ENV_FILE=".env.local"
DEPLOYMENT_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    prod)
      DEPLOYMENT_ARGS=(--prod)
      shift
      ;;
    --deployment-name)
      [[ $# -ge 2 ]] || { echo "Error: --deployment-name requires a value" >&2; exit 1; }
      DEPLOYMENT_ARGS=(--deployment-name "$2")
      shift 2
      ;;
    --env-file)
      [[ $# -ge 2 ]] || { echo "Error: --env-file requires a value" >&2; exit 1; }
      ENV_FILE="$2"
      shift 2
      ;;
    *)
      echo "Error: unsupported argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Error: $ENV_FILE file not found" >&2
  exit 1
fi

get_env() {
  local key="$1"
  local value
  value="$(sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n 1)"
  if [[ "$value" =~ ^\"(.*)\"$ ]]; then
    value="${BASH_REMATCH[1]}"
  elif [[ "$value" =~ ^\'(.*)\'$ ]]; then
    value="${BASH_REMATCH[1]}"
  fi
  printf '%s' "$value"
}

export CONVEX_SELF_HOSTED_URL="$(get_env CONVEX_SELF_HOSTED_URL)"
export CONVEX_SELF_HOSTED_ADMIN_KEY="$(get_env CONVEX_SELF_HOSTED_ADMIN_KEY)"

if [[ -z "$CONVEX_SELF_HOSTED_URL" || -z "$CONVEX_SELF_HOSTED_ADMIN_KEY" ]]; then
  echo "Error: self-hosted Convex URL or admin key is missing from $ENV_FILE" >&2
  exit 1
fi

FUNCTION_ENV=(SITE_URL JWT_PRIVATE_KEY JWKS)

for name in "${FUNCTION_ENV[@]}"; do
  value="$(get_env "$name")"
  if [[ -z "$value" ]]; then
    echo "Error: $name is missing from $ENV_FILE" >&2
    exit 1
  fi

  if output="$(printf '%s' "$value" | npx convex env set "${DEPLOYMENT_ARGS[@]}" -- "$name" 2>&1)"; then
    echo "Set $name"
  else
    output="${output//$value/<redacted>}"
    echo "$output" >&2
    exit 1
  fi
done

echo "Pushed the allowlisted function environment to Convex."
