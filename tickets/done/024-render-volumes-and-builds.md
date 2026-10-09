# 024 - Honor volume types and built image relationships

## Problem

Volume rendering can select the wrong data location, and built services can reference image tags their build never produces.

## Work

- Render volumes in explicit long syntax.
- Remove unused destination naming or implement one clear route destination contract.
- Validate each built service image against its build tags.
- Allow upstream-only deployments without builder setup.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should services infer the image from their build?

Default: Validate the existing explicit image/tag relationship to preserve configuration compatibility.
