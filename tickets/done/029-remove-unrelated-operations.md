# 029 - Remove unrelated operational code and records

## Problem

The repository contains app-specific operational records, obsolete platform provisioning and one-off migration tools.

## Work

- Move app-specific records and obsolete research outside the repository.
- Remove legacy Coolify provisioning and refuse its old enable flag.
- Move the one-off repository migration script and specification outside pp.
- Keep generic pp tickets and reusable decisions.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should old operational material be deleted permanently?

Default: Preserve a copy outside pp, then remove it from the branch.
