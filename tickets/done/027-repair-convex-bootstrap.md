# 027 - Separate Convex initialization, rotation and recovery

## Problem

Address audit findings 7, 36, 37 with generic pp behavior and reusable examples.

## Work

- Preserve app-owned env values during initialization and explicit rotation.
- Keep recovery copies until verification and replace files atomically.
- Use explicit production env files and locally installed Convex tooling.
- Give backend storage a restricted MinIO identity.
- Document a working private dashboard access path.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should rerunning bootstrap rotate credentials?

Default: Initialization reuses existing credentials; rotation requires an explicit flag.
