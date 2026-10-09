# 014 - Allocate project backend ports

## Goal

Avoid port collisions between projects by letting `pp` allocate stable backend ports on each host.

## Scope

- Allow `published: auto` in service port config.
- Allocate from reserved host-local range `18000-19999`.
- Store stable host allocation state under `/etc/pp/ports.tsv`.
- Lock allocation with `/etc/pp/ports.lock`.
- Avoid ports already reported by `ss -ltn`.
- Render compose with allocated `127.0.0.1:<port>:<target>` bindings.
- Allow Caddy routes to infer targets from service ports.

## Acceptance

- Existing fixed ports still render unchanged.
- Auto ports are stable per `project:service:target` key.
- Caddy routes can omit explicit `target` when the service has one matching port.
- Validate allocation using isolated host-command fixtures.
