# 028 - Make examples portable and maintainable

## Problem

Address audit findings 6, 38 with generic pp behavior and reusable examples.

## Work

- Use reviewed upstream version pins and a documented update process.
- Run the Go runtime without root and build for the selected target architecture.
- Remove blanket dependency build-script approval.
- Document persistent config and SQLite migration behavior.
- Add reusable backup and isolated restore instructions for stateful examples.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should examples upgrade infrastructure images on every app deploy?

Default: Keep pinned versions; infrastructure upgrades are deliberate edits and reviewed releases.
