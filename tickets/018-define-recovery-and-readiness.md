# 018 - Define database recovery and service readiness

## Problem

Address audit findings 16, 17, 22 with generic pp behavior and reusable examples.

## Work

- Share an explicit database-before-code recovery sequence.
- Wait for configured service health before hooks and success.
- Use bounded smoke retries and step timeouts.
- Propagate deployment cancellation to HTTP, DNS, Git and subprocesses.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should pp automatically restore arbitrary databases?

Default: Run a declared rollback command; stop and report manual recovery for restore plans or partial migration failures.
