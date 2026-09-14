# Security review

Source review from 2026-09-07, commit `7dff342` plus working-tree changes. Updated 2026-09-14. Assumes trusted deployment configuration.

Findings 1 and 2 are fixed in the CLI. Findings 3 through 6 remain open.

1. **Fixed: dev and prod shared resources.** New deployments use project/environment resource names. Legacy state upgrades automatically, preserving existing names with checked ownership. State, shutdown and rollback remain environment-scoped. Bind mounts, external volumes and helper scripts still need separate environment configuration. [Migration guide](deploy-cli.md#migrating-an-existing-deployment)

2. **Fixed: fixed ports defaulted to public access.** Fixed and automatic ports now default to `127.0.0.1`; public bindings require an explicit `host_ip`. Humanist already uses loopback behind host-level Caddy, and all seven live smoke checks passed on 2026-09-14. Existing fixed ports change on their next deployment. [Code](internal/deploy/compose.go#L378)

3. **Medium: uploaded secrets can be readable by other host users.** Rendered Compose files contain passwords and use `0644`; remote directories have no enforced private mode. Exposure depends on home-directory permissions and umask. Enforce `0700` directories and `0600` secret files. [Code](internal/deploy/operations.go#L497)

4. **Medium: services receive unrelated secrets.** `env.required` validates presence, but the renderer copies every source key. The Convex example gives Postgres and MinIO each other's credentials and the Convex instance secret. Use separate service env files or explicit key selection. [Code](internal/deploy/compose.go#L349)

5. **Medium: health-check URLs execute shell syntax.** The URL is appended unquoted to `CMD-SHELL`. A less-trusted interpolated value containing `;command` can execute inside the container. Use a `CMD` argument array and validate the rendered URL. Someone controlling the whole config already has command execution. [Code](internal/deploy/compose.go#L327)

6. **Medium: Compose changes literal passwords.** Compose reinterprets `$NAME` and `${NAME}` after the CLI renders credentials. A dummy `prefix${UNSET}suffix` became `prefixsuffix`, risking failed authentication or unintended passwords. Escape literal dollars in YAML and preserve literal env-file values. [Code](internal/deploy/compose.go#L416) · [Compose behavior](https://docs.docker.com/reference/compose-file/interpolation/)

Validation: Go tests and vet passed. Humanist's migration, seven live smoke checks and a Convex WebSocket subscription passed on 2026-09-14. Findings 5 and 6 were reproduced locally. Dependency and image vulnerability scans remain outstanding.
