# 031 - Simplify shared execution code

## Problem

Host preparation is duplicated. Recovery reparses saved inputs, image records accumulate duplicates, and cancellation can leave migration containers running.

## Work

- Share host preparation and recovery paths after behavior changes land.
- Remove unused rendering arguments, conversions and Docker restart handlers.
- Load optional host credentials before validating role flags; reject malformed credential files.
- Deduplicate image manifest entries.
- Align the Go module with the public repository name.
- Validate required route and image-lock artifacts before database recovery. Refuse using a database undo command to move to a newer release.
- Remove owned migration containers after timeout and report cleanup errors.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should this introduce a deployment framework?

Default: Keep the current packages and small helpers.
