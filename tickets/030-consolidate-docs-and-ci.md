# 030 - Consolidate documentation and focused validation

## Problem

Address audit findings 40, 42 with generic pp behavior and reusable examples.

## Work

- Publish one implemented config reference and current CLI, recovery, security and developer guides.
- Keep first-time setup front and center in README.
- Add a small CI job with unit, race, vet, syntax and vulnerability checks.
- Document isolated Docker tests and Ansible collection dependencies.
- Apply unslop to all retained documentation.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should validation use sibling applications or live production hosts?

Default: Use repository-owned isolated fixtures and explicit opt-in container tests.
