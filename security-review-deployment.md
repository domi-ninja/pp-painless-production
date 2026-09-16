# Deployment Security Review

Scope: obvious security gaps in the new deployment system, including the Go deploy CLI and Ansible deployment/CI roles.

## Findings

### High: CI runners can become host compromise paths

The Forgejo runner starts privileged Docker-in-Docker with unauthenticated TCP Docker on port 2375, and the Woodpecker agent mounts `/var/run/docker.sock`. Any workflow that reaches these agents can control Docker and likely root-equivalent host resources.

References:
- `infra/ansible/roles/forgejo_runner/templates/compose.yml.j2:4`
- `infra/ansible/roles/forgejo_runner/templates/compose.yml.j2:8`
- `infra/ansible/roles/woodpecker_agent/templates/compose.yml.j2:8`

Recommendation: run these only on isolated runner VMs, not shared production hosts. Restrict deployment workflows to trusted refs, keep runner scope narrow, and avoid host Docker socket access for untrusted jobs.

### Fixed: SSH target option injection

Config validation and every deployment SSH entrypoint reject options, whitespace and shell syntax. Use `user@hostname` or an SSH config alias; configure ports and jump hosts in SSH config. Original finding follows.

`hosts.*.ssh` is only checked for non-empty, then passed directly to `ssh` and `scp`. A malicious config value beginning with SSH options, such as a `ProxyCommand`, can turn deploy into local command execution on the deploy machine.

References:
- `internal/deploy/config.go:168`
- `internal/deploy/operations.go:235`
- `internal/deploy/operations.go:239`

Recommendation: model SSH as structured fields such as `user`, `host`, and `port`, or strictly reject values beginning with `-` and invoke `ssh`/`scp` with `--` before the target where supported.

### Fixed: remote secret env files are permissioned after upload

Release directories are now `0700` before transfer; Compose and env files are `0600` afterward. Tests cover new and reused paths. The original finding follows.

Local rendered env files are written as `0600`, but remote upload uses plain `scp` into directories created by `mkdir -p` with default remote permissions. There is no remote `chmod` or `install -m 0600`.

References:
- `internal/deploy/compose.go:235`
- `internal/deploy/operations.go:198`
- `internal/deploy/operations.go:210`

Recommendation: create remote env directories as `0700`, upload to a temporary path, then install env files with `0600`. Consider removing stale env files from previous releases if secrets rotate.

### Fixed: services receive only declared env keys

Service env files now use `env.required` as an allowlist. Whole-file injection requires `include_all: true`. Tests verify that database and storage services do not receive each other's secrets. The original finding follows.

`env.required` is validation-only. When a service has an env source, every key from that source file is rendered into that service's env file, including secrets the service did not declare.

References:
- `internal/deploy/config.go:277`
- `internal/deploy/compose.go:220`

Recommendation: render only declared keys, or introduce an explicit allowlist field. If whole-file behavior is desired, require a deliberate opt-in such as `include_all: true`.

### Medium: Caddy route rendering allows config injection

`route.host` and `route.target` are written directly into Caddy config. `target` only gets URL parsing and `host` is not syntax-validated, so crafted committed config can inject arbitrary Caddy directives before reload.

References:
- `internal/deploy/compose.go:142`
- `internal/deploy/compose.go:148`
- `internal/deploy/compose.go:150`
- `internal/deploy/config.go:217`

Recommendation: reject control characters, braces, whitespace, and newlines in route host/target fields. Validate route hosts as DNS names or explicit supported wildcard patterns.

### Medium: config paths can escape the repository boundary

`env.source` is joined and opened without rejecting `..` or absolute paths. Docker build context and Dockerfile are only checked for non-empty, not constrained to the repository.

References:
- `internal/deploy/config.go:154`
- `internal/deploy/config.go:272`
- `internal/deploy/operations.go:124`
- `internal/deploy/operations.go:134`

Recommendation: require relative, clean, in-repo paths for env sources, build contexts, and Dockerfiles unless an explicit escape hatch is added.

## Additional Notes

Ignored local credential files are present in the working tree, including `scripts/.master-token`, but `git check-ignore` confirms that file is ignored and `git ls-files` did not show it as tracked. Secret contents were not inspected.

Verification performed during review:

```sh
go test ./...
```

Result: passed.
