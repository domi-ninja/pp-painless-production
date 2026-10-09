# 024 - Honor volume types and built image relationships

## Problem

Address audit findings 27, 28 with generic pp behavior and reusable examples.

## Work

- Render volumes in explicit long syntax.
- Remove unused destination naming or implement one clear route destination contract.
- Validate each built service image against its build tags.
- Allow upstream-only deployments without builder setup.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should services infer the image from their build?

Default: Validate the existing explicit image/tag relationship to preserve configuration compatibility.
