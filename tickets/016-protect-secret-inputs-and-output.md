# 016 - Protect secret inputs and diagnostic output

## Problem

Address audit findings 2, 3, 4, 12 with generic pp behavior and reusable examples.

## Work

- Use declared env keys consistently for services, hooks, checks and migrations.
- Remove arguments and sensitive URLs from logs and persisted execution errors.
- Protect state and init destinations against symlinks.
- Keep route templates public; reject sensitive URL credentials.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should hooks retain whole-file env injection?

Default: Require include_all for whole-file input; preserve ordinary tool environment.
