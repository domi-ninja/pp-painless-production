# 001 - Define deployment config schema

## Goal

Finalize `deploy.yml` v1 as the committed desired-state format for one project.

## Dependencies

- [Current configuration reference](../../docs/configuration.md)
- [Current deployment behavior](../../docs/deploy-cli.md)

## Scope

- Convert the draft schema into a precise v1 spec.
- Decide required vs optional fields.
- Decide template variables: `${git_sha}`, `${project}`, `${environment}`, `${release}`.
- Document how env values are provisioned into runtime containers.
- Document migration, DB rollback, health check, volume, port, and host placement semantics.
- Lock release ID format as timestamp plus git SHA.
- Lock dirty worktree metadata behavior.

## Deliverables

- Versioned schema spec document.
- Minimal valid example.
- Maximal example covering web, worker, volume, migration, DB rollback, and smoke check.
- List of rejected/non-goals for v1.

## Acceptance

- A future parser can be implemented without guessing field behavior.
- The schema supports one image deployed to multiple hosts.
- Cross-host networking is explicitly URL/DNS-based, not Docker-network-based.
- Secrets are not represented directly in committed config.
- Dirty deploys are allowed but traceable in metadata.
- Prod migrations run inside the built app image.
