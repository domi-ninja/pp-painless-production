#!/usr/bin/env bash
set -Eeuo pipefail

# Use the upstream initialization helpers. Existing volumes need their role
# password updated too; POSTGRES_PASSWORD alone only affects fresh volumes.
source /usr/local/bin/docker-entrypoint.sh
docker_setup_env
docker_create_db_directories
if [[ "$(id -u)" == 0 ]]; then
  exec gosu postgres bash "$0" "$@"
fi

if [[ -n "$DATABASE_ALREADY_EXISTS" ]]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
  docker_temp_server_start "$@" >/dev/null
  trap 'docker_temp_server_stop >/dev/null' EXIT
  # psql reads credentials from the environment, never from command arguments.
  # The temporary server only listens on its Unix socket, not the network.
  psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" >/dev/null <<'SQL'
\getenv role_name POSTGRES_USER
\getenv role_password POSTGRES_PASSWORD
SELECT format('ALTER ROLE %I PASSWORD %L', :'role_name', :'role_password') \gexec
SQL
  docker_temp_server_stop >/dev/null
  trap - EXIT
fi

exec /usr/local/bin/docker-entrypoint.sh "$@"
