# 026 - Make the starter and operation preview useful

## Problem

Address audit findings 32, 35, 39 with generic pp behavior and reusable examples.

## Work

- Generate a real local health check and safe Docker ignore rules.
- Use release-specific starter image tags.
- Show images, routes, mounts, env key scopes and recovery policy in plan.
- Add version/build identity and useful command help.
- Document dirty-build digest limits.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should the starter silently accept a placeholder health check?

Default: Use a localhost HTTP check and tell the operator to match its endpoint and port.
