# 026 - Make the starter and operation preview useful

## Problem

The starter's health check always succeeds, the plan omits useful inputs, and a local build can replace the installed CLI.

## Work

- Generate a real local health check and safe Docker ignore rules.
- Use release-specific starter image tags.
- Show images, routes, mounts, env key scopes and recovery policy in plan.
- Add version/build identity and useful command help.
- Document dirty-build digest limits.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should the starter silently accept a placeholder health check?

Default: Use a localhost HTTP check and tell the operator to match its endpoint and port.
