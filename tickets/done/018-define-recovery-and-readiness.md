# 018 - Define database recovery and service readiness

## Problem

Readiness differs between phases, and recovery can start older containers before reverting the database.

## Work

- Share an explicit database-before-code recovery sequence.
- Wait for configured service health before hooks and success.
- Use bounded smoke retries and step timeouts.
- Propagate deployment cancellation to HTTP, DNS, Git and subprocesses.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should pp automatically restore arbitrary databases?

Default: Run a declared rollback command; stop and report manual recovery for restore plans or partial migration failures.
