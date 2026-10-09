# 020 - Make the supplied host setup usable

## Problem

Address audit findings 34 with generic pp behavior and reusable examples.

## Work

- Provision the documented deployment account, pp paths and Caddy permissions.
- Keep CLI and provisioning route paths consistent.
- Add tool, permission and architecture preflight before build or migration.
- Verify the existing-host instructions using isolated fixtures.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Which host account should pp support by default?

Default: Use deploy with Docker access and owned pp paths; Caddy reload uses its local admin endpoint.
