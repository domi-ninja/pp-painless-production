# 007 - Build Go host agent MVP

## Goal

Create the persistent host management agent as a Go binary.

## Dependencies

- 001

## Scope

- Implement a small local HTTP API reached through SSH.
- Report host ID, agent version, disk space, loaded image tags, compose status, container health, and last deploy metadata.
- Persist agent state to a host-local JSON or SQLite file.
- Read Docker state using Docker CLI or Docker API.
- Keep apply actions out of the agent for v1; SSH remains apply transport.

## Deliverables

- Go agent source.
- API contract document.
- Systemd-compatible binary behavior.
- Unit tests for state persistence and response shapes.

## Acceptance

- CLI can query agent health and host state.
- Agent restart preserves last deploy metadata.
- Agent exposes no unauthenticated mutating endpoints.
- Agent has a version endpoint for compatibility checks.
- Agent binds only to localhost or another explicitly local interface.
