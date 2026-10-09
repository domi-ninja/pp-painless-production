# 016 - Protect secret inputs and diagnostic output

## Problem

Runtime inputs share too many env keys, diagnostics can expose credentials, and symlink writes can escape private state.

## Work

- Use declared env keys consistently for services, hooks, checks and migrations.
- Remove arguments and sensitive URLs from logs and persisted execution errors.
- Protect state and init destinations against symlinks.
- Keep route templates public; reject sensitive URL credentials.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should hooks retain whole-file env injection?

Default: Require include_all for whole-file input; preserve ordinary tool environment.
