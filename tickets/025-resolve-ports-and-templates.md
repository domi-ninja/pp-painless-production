# 025 - Resolve ports, templates and env values consistently

## Problem

Address audit findings 30, 31 with generic pp behavior and reusable examples.

## Work

- Reserve pending fixed ports during automatic allocation and validate resolved collisions.
- Check saved port ownership and support deliberate retirement.
- Apply documented templates to command and entrypoint arguments.
- Reject unresolved pp placeholders and use one documented env parser.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should cleanup automatically reclaim retired ports?

Default: Keep allocation retirement explicit; ordinary release retention does not reclaim ports.
