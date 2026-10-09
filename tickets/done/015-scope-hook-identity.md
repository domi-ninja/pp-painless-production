# 015 - Pass deployment identity to hooks

## Problem

Hooks can select another environment's containers when they match only the project name.

## Work

- Give hooks the selected config, project, environment and host identities.
- Select helper containers by project, environment and service; reject zero or multiple matches.
- Test dev/prod selection and alternate config filenames.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should helpers infer an environment when run by hand?

Default: Require explicit identity; never fall back to another environment.
