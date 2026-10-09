# 022 - Make storage changes recoverable

## Problem

Docker relocation can copy the wrong source or discard daemon settings. A missing data mount can start an empty database.

## Work

- Copy Docker state from its actual current root.
- Reject overlapping paths and recover original Docker configuration on failure.
- Render Docker daemon settings once.
- Require the expected data mount before stateful service startup.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should storage relocation run without an explicit backup declaration?

Default: Require backup confirmation in configuration and preserve original data.
