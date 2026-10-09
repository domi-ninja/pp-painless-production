# 025 - Resolve ports, templates and env values consistently

## Problem

Automatic ports can collide with pending fixed ports. Template expansion and env parsing can alter literal values.

## Work

- Reserve pending fixed ports during automatic allocation and validate resolved collisions.
- Check saved port ownership and support deliberate retirement.
- Apply documented templates to command and entrypoint arguments.
- Reject unresolved pp placeholders and use one documented env parser.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should cleanup automatically reclaim retired ports?

Default: Keep allocation retirement explicit; ordinary release retention does not reclaim ports.
