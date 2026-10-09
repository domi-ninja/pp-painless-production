# 020 - Make the supplied host setup usable

## Problem

The deployment account needs writable pp paths. Missing tools and host incompatibility must fail before migration.

## Work

- Provision the documented deployment account, pp paths and Caddy permissions.
- Keep CLI and provisioning route paths consistent.
- Add tool, permission and architecture preflight before build or migration.
- Verify the existing-host instructions using isolated fixtures.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Which host account should pp support by default?

Default: Use deploy with Docker access and owned pp paths; Caddy reload uses its local admin endpoint.
