# 012 - Recover from the saved release contract

## Problem

Rollback currently combines an old Compose bundle with current configuration, hooks and migration commands. Current secrets or Git inputs can block recovery.

## Work

- Save a versioned execution contract in each release, including phases, checks and migration inputs.
- Recover from saved deployment inputs rather than current app scripts.
- Verify all required artifacts before changing services or data.
- Reject legacy recovery that would require guessing a migration or rerunning app-changing hooks.
- Let inspection and recovery load identity without validating unrelated build inputs.

## Acceptance

Focused tests cover changed config, missing current env, missing artifacts and legacy release recovery. Hook-driven application changes require an explicit recovery path.

## Decision

Should rollback rerun current application hooks?

Default: never rerun current hooks; recover containers and saved checks, and require explicit recovery for hook-managed application changes.
