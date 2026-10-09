# 017 - Use a patched toolchain and document trust boundaries

## Problem

The CLI needs a patched compiler and a documented boundary between trusted deployment code and secret inputs.

## Work

- Build with a supported patched Go release and rerun vulnerability checks.
- Replace historical security claims with current operator, secret and firewall boundaries.
- Remove obsolete dangerous copy-paste instructions from user documentation.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should pp promise tenant isolation?

Default: Document trusted operator access and host-wide Docker authority.
