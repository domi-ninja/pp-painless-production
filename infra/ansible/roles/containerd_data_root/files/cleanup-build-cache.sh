#!/usr/bin/env bash
# Only disposable, unused build cache. Keep images, containers and volumes.
set -euo pipefail
exec 9>/run/pp-build-cache-cleanup.lock
flock -n 9 || exit 0
docker builder prune --force --filter until=168h --keep-storage=10GB
