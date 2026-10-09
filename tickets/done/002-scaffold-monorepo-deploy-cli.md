# 002 - Scaffold deployment CLI

## Goal

Create the Go monorepo CLI package that exposes the deployment command surface.

## Dependencies

- 001

## Scope

- Add the CLI package in this repo.
- Implement it in Go so deployment code is separated from project runtime languages.
- Implement command routing for:
  - `deploy`
  - `deploy plan`
  - `deploy status`
  - `deploy rollback`
- Add shared logging, error formatting, and project root discovery.
- Support default config path `deploy.yml`.
- Keep normal deploy param-less; flags are diagnostics and emergency overrides only.

## Deliverables

- Go CLI entrypoint.
- Command stubs with structured output.
- Basic unit tests for command parsing.
- README snippet showing intended invocation.

## Acceptance

- Running `deploy plan` from a project root finds `deploy.yml`.
- Running `deploy` without args enters the default full deployment path.
- Missing config returns a clear actionable error.
- CLI package is usable from multiple side-project repos.
- CLI does not require project language dependencies to run.
