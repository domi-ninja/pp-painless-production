# 030 - Consolidate documentation and focused validation

## Problem

The docs describe conflicting behavior and omit implemented fields. Validation needs isolated fixtures and declared dependencies.

## Work

- Publish one implemented config reference and current CLI, recovery, security and developer guides.
- Keep first-time setup front and center in README.
- Add a small CI job with unit, race, vet, syntax and vulnerability checks.
- Document isolated Docker tests and Ansible collection dependencies.
- Apply unslop to all retained documentation.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should validation use sibling applications or live production hosts?

Default: Use repository-owned isolated fixtures and explicit opt-in container tests.
