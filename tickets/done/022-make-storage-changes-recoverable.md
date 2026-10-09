# 022 - Make storage changes recoverable

## Problem

Address audit findings 23, 24 with generic pp behavior and reusable examples.

## Work

- Copy Docker state from its actual current root.
- Reject overlapping paths and recover original Docker configuration on failure.
- Render Docker daemon settings once.
- Require the expected data mount before stateful service startup.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should storage relocation run without an explicit backup declaration?

Default: Require backup confirmation in configuration and preserve original data.
