# 017 - Use a patched toolchain and document trust boundaries

## Problem

Address audit findings 5, 8, 11, 41 with generic pp behavior and reusable examples.

## Work

- Build with a supported patched Go release and rerun vulnerability checks.
- Replace historical security claims with current operator, secret and firewall boundaries.
- Remove obsolete dangerous copy-paste instructions from user documentation.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should pp promise tenant isolation?

Default: Document trusted operator access and host-wide Docker authority.
