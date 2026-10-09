# 004 - Render per-host Compose Bundles

## Goal

Generate deterministic per-host Docker Compose bundles from the logical deployment config.

## Dependencies

- 003

## Scope

- Select services per host using `services.*.hosts`.
- Render one `compose.yml` per host.
- Render runtime env provisioning artifacts into per-service host bundle paths.
- Include labels for project, environment, release, service, git SHA, and deploy ID.
- Generate `.deploy/releases/<release-id>/` bundle layout.

## Deliverables

- Compose renderer.
- Bundle writer.
- Snapshot tests for generated bundles.
- `deploy plan` output showing generated host/service mapping.

## Acceptance

- A service assigned to two hosts appears in both host compose files.
- Host-local ports are checked for conflicts before rendering succeeds.
- Rendered compose files do not contain raw local file paths except intended bind mounts or host-local env artifacts.
- Bundle output is stable for identical inputs.
