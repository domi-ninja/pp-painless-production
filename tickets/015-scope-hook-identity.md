# 015 - Pass deployment identity to hooks

## Problem

Address audit findings 1, 3, 37 with generic pp behavior and reusable examples.

## Work

- Give hooks the selected config, project, environment and host identities.
- Select helper containers by project, environment and service; reject zero or multiple matches.
- Test dev/prod selection and alternate config filenames.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should helpers infer an environment when run by hand?

Default: Require explicit identity; never fall back to another environment.
