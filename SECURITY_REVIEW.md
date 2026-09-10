# Security review

Source review from 2026-09-07, commit `7dff342` plus working-tree changes. Shortened 2026-09-10; findings were not re-audited. Assumes trusted deployment configuration.

Fix the two high-severity issues first.

1. **High: dev and prod share resources.** With the same project name on one host, environments reuse Compose resources, allocated ports and route files. Dev can replace prod containers; `down` selects both. Local state also mixes environments in one checkout. Include project and environment in resource identities, state and removal filters. Existing resources need migration. [Code](internal/deploy/operations.go#L328)

2. **High: fixed ports default to public access.** Changing `published: auto` to a number drops the loopback binding unless `host_ip` is explicit. Reachable clients can bypass proxy TLS and access controls. Default all ports to `127.0.0.1`; require an explicit public binding. [Code](internal/deploy/compose.go#L378) · [Docker behavior](https://docs.docker.com/engine/network/port-publishing/)

3. **Medium: uploaded secrets can be readable by other host users.** Rendered Compose files contain passwords and use `0644`; remote directories have no enforced private mode. Exposure depends on home-directory permissions and umask. Enforce `0700` directories and `0600` secret files. [Code](internal/deploy/operations.go#L484)

4. **Medium: services receive unrelated secrets.** `env.required` validates presence, but the renderer copies every source key. The Convex example gives Postgres and MinIO each other's credentials and the Convex instance secret. Use separate service env files or explicit key selection. [Code](internal/deploy/compose.go#L349)

5. **Medium: health-check URLs execute shell syntax.** The URL is appended unquoted to `CMD-SHELL`. A less-trusted interpolated value containing `;command` can execute inside the container. Use a `CMD` argument array and validate the rendered URL. Someone controlling the whole config already has command execution. [Code](internal/deploy/compose.go#L327)

6. **Medium: Compose changes literal passwords.** Compose reinterprets `$NAME` and `${NAME}` after the CLI renders credentials. A dummy `prefix${UNSET}suffix` became `prefixsuffix`, risking failed authentication or unintended passwords. Escape literal dollars in YAML and preserve literal env-file values. [Code](internal/deploy/compose.go#L416) · [Compose behavior](https://docs.docker.com/reference/compose-file/interpolation/)

Original checks: `go test ./...` and `go vet ./...` passed. Local checks reproduced shell execution and password interpolation. No live infrastructure was tested or changed. Dependency and image vulnerability scans remain outstanding.
