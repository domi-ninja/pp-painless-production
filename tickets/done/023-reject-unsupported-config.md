# 023 - Reject unsupported deployment configuration

## Problem

Unknown fields and unsupported values can be silently ignored, leaving users with a different deployment than they configured.

## Work

- Reject unknown nested YAML fields.
- Validate supported service phases, hook phases and check groups.
- Validate secrets, paths, ports, host addresses and timeouts.
- Reject unsupported multiple platforms and meaningless host policy fields.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should roles enforce an access policy?

Default: Keep descriptive roles but reject CI placement and document the host marker guard.
