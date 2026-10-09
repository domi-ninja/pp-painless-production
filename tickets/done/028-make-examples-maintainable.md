# 028 - Make examples portable and maintainable

## Problem

Examples use mutable image references, a root Go runtime and blanket dependency-script approval. Stateful updates need backup and restore guidance.

## Work

- Use reviewed upstream version pins and a documented update process.
- Run the Go runtime without root and build for the selected target architecture.
- Remove blanket dependency build-script approval.
- Document persistent config and SQLite migration behavior.
- Add reusable backup and isolated restore instructions for stateful examples.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should examples upgrade infrastructure images on every app deploy?

Default: Keep pinned versions; infrastructure upgrades are deliberate edits and reviewed releases.
