# 031 - Simplify shared execution code

## Problem

Address audit findings 33 with generic pp behavior and reusable examples.

## Work

- Share host preparation and recovery paths after behavior changes land.
- Remove unused rendering arguments and handwritten conversions.
- Deduplicate image manifest entries.
- Align the Go module with the public repository name.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should this introduce a deployment framework?

Default: Keep the current packages and small helpers.
