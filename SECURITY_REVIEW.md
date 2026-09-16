# Security review

Source review from 2026-09-07, commit `7dff342` plus working-tree changes. Updated 2026-09-14. Assumes trusted deployment configuration.

Findings 1 through 5 are fixed in the CLI. Finding 6 remains open.

1. **Fixed: dev and prod shared resources.** New deployments use project/environment resource names. Legacy state upgrades automatically, preserving existing names with checked ownership. State, shutdown and rollback remain environment-scoped. Bind mounts, external volumes and helper scripts still need separate environment configuration. [Migration guide](deploy-cli.md#migrating-an-existing-deployment)

2. **Fixed: fixed ports defaulted to public access.** Fixed and automatic ports now default to `127.0.0.1`; public bindings require an explicit `host_ip`. Humanist already uses loopback behind host-level Caddy, and all seven live smoke checks passed on 2026-09-14. Existing fixed ports change on their next deployment. [Code](internal/deploy/compose.go#L378)

3. **Fixed: secret-file permissions.** Local Compose files use `0600`. Remote release and env directories are restricted to `0700` before upload, and Compose/env files to `0600` afterward, including reused paths. Existing releases are not retroactively changed until uploaded again.

4. **Fixed: unrelated service secrets.** Service env files contain only `env.required` keys. Copying the whole source requires explicit `env.include_all: true`. Existing configs with a service env source but no key list must declare one before deploying. Build and hook env sources remain operator-controlled inputs.

5. **Fixed: HTTP health-check shell injection.** The rendered URL is validated and passed as one argument in a `CMD` array, after `--`. An execution test proves the old shell form creates a marker file with a `;touch${IFS}` payload, while the new check succeeds without creating it. Explicit command health checks remain trusted configuration.

6. **Medium: Compose changes literal passwords.** Compose reinterprets `$NAME` and `${NAME}` after the CLI renders credentials. A dummy `prefix${UNSET}suffix` became `prefixsuffix`, risking failed authentication or unintended passwords. Escape literal dollars in YAML and preserve literal env-file values. [Code](internal/deploy/compose.go#L416) · [Compose behavior](https://docs.docker.com/reference/compose-file/interpolation/)

Validation: Go tests and vet passed. Humanist's migration, seven live smoke checks and a Convex WebSocket subscription passed on 2026-09-14. Findings 5 and 6 were reproduced locally. Dependency and image vulnerability scans remain outstanding.
