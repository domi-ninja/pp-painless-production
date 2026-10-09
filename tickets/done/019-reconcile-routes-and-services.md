# 019 - Apply routes safely and retire stale services

## Problem

A failed Caddy reload can leave changed routes in place. Removed services and routes also need explicit reconciliation.

## Work

- Stage, validate and recover shared Caddy configuration changes.
- Remove retired owned services without deleting data volumes.
- Remove obsolete deployment routes and detect conflicting domains.
- Refuse implicit retirement of a removed host; require a deliberate shutdown first.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should removing a host silently shut it down?

Default: Refuse until its owned services have been explicitly stopped.
