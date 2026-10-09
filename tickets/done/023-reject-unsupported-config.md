# 023 - Reject unsupported deployment configuration

## Problem

Address audit findings 25, 26, 29 with generic pp behavior and reusable examples.

## Work

- Reject unknown nested YAML fields.
- Validate supported service phases, hook phases and check groups.
- Validate secrets, paths, ports, host addresses and timeouts.
- Reject unsupported multiple platforms and meaningless host policy fields.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should roles enforce an access policy?

Default: Keep descriptive roles but reject CI placement and document the host marker guard.
