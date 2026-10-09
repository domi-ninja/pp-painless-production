# 027 - Separate Convex initialization, rotation and recovery

## Problem

Bootstrap reruns can rotate credentials and drop app-owned keys. Helpers can use development inputs or storage admin credentials.

## Work

- Preserve app-owned env values during initialization and explicit rotation.
- Keep recovery copies until verification and replace files atomically.
- Use explicit production env files and locally installed Convex tooling.
- Give backend storage a restricted identity. The replacement storage implementation is covered by ticket 028.
- Document a working private dashboard access path.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should rerunning bootstrap rotate credentials?

Default: Initialization reuses existing credentials; rotation requires an explicit flag.
