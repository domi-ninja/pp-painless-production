# 029 - Remove unrelated operational code and records

## Problem

Address audit findings 13, 14, 41 with generic pp behavior and reusable examples.

## Work

- Move app-specific records and obsolete research outside the repository.
- Remove legacy Coolify provisioning and refuse its old enable flag.
- Move the one-off repository migration script and specification outside pp.
- Keep generic pp tickets and reusable decisions.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should old operational material be deleted permanently?

Default: Preserve a copy outside pp, then remove it from the branch.
