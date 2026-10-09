# 019 - Apply routes safely and retire stale services

## Problem

Address audit findings 18, 19 with generic pp behavior and reusable examples.

## Work

- Stage, validate and recover shared Caddy configuration changes.
- Remove retired owned services without deleting data volumes.
- Remove obsolete deployment routes and detect conflicting domains.
- Refuse implicit retirement of a removed host; require a deliberate shutdown first.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should removing a host silently shut it down?

Default: Refuse until its owned services have been explicitly stopped.
