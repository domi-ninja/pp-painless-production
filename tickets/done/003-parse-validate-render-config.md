# 003 - Parse, Validate, And Render Config

## Goal

Implement config loading, schema validation, env validation, and template rendering.

## Dependencies

- 001
- 002

## Scope

- Parse `deploy.yml`.
- Validate schema version and required fields.
- Validate host, service, volume, port, migration, and check references.
- Load deploy-time env sources without printing secret values.
- Validate required runtime env names before touching prod.
- Render supported template variables deterministically.
- Record git SHA, dirty flag, and worktree diff digest in the rendered model.
- Produce an in-memory desired-state model for later steps.

## Deliverables

- Config parser module.
- Validator module.
- Template rendering module.
- Test fixtures for valid and invalid configs.

## Acceptance

- Unknown required references fail before build/apply.
- Missing env sources or required env values fail before touching prod.
- Same config, env, and git SHA produce identical rendered output.
- Validation errors include field paths.
- Dirty worktrees are accepted but recorded.
