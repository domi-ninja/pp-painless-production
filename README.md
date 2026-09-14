# Codeberg CI/CD and `pp` Deployment Research

This repository contains two related but separate tracks:

1. Ansible roles for preparing production servers and optional platform services such as Forgejo.
2. The new `pp` deployment system, a Go CLI that deploys application releases to already-prepared Docker hosts.

The important boundary is that Ansible prepares the host, while `pp` deploys application releases.

## Repository Layout

```text
cmd/deploy/                  Go entrypoint for the `pp` deploy CLI
internal/deploy/             `pp` config parsing, planning, rendering, transfer, apply, status, rollback
examples/                    Deployment config snapshots for common application shapes
infra/ansible/               Production server provisioning playbooks and roles
deployment-*.md              Design notes for the deployment system
deploy-cli.md                Current CLI behavior
tickets/                     Implementation tickets for the `pp` system
```

## Ansible: Server And Platform Provisioning

The Ansible stack under `infra/ansible/` is for configuring long-lived server state:

- Base Ubuntu hardening: users, SSH, UFW, fail2ban, unattended upgrades, time sync, sysctl, logs.
- Docker installation and optional Docker data-root relocation.
- Persistent data volume mounting, normally under `/data`.
- Optional platform services such as Forgejo, Forgejo Actions runner, and Woodpecker agent.
- Optional host-level reverse proxy setup.

Run it from `infra/ansible/`:

```sh
ansible-playbook -i inventory/prod.local.yml playbooks/prod-server.yml
```

Host-specific values belong in ignored local inventory or host vars files, for example:

```text
infra/ansible/inventory/prod.local.yml
infra/ansible/host_vars/<host>.yml
infra/ansible/host_vars/<host>.credentials.local.yml
```

Secrets should stay in ignored files or Ansible Vault, not in committed config.

### Forgejo Ansible Path

Forgejo is platform infrastructure. Use the Ansible `forgejo` role when the goal is to run or maintain the Git forge service itself.

That role owns:

- `/data/forgejo`
- the `forgejo` and `forgejo-db` containers
- Forgejo HTTP and Git SSH port exposure
- Forgejo application secrets and first-admin bootstrap
- optional Caddy proxy-route integration through the host-level reverse proxy role

Forgejo is not deployed by `pp`. It is a persistent service managed by Ansible because it is part of the server platform, not a side-project release.

On hosts that should expose Forgejo on HTTPS, enable the reverse proxy and Forgejo role together:

```yaml
reverse_proxy_enabled: true
forgejo_enabled: true
forgejo_domain: git.example.com
forgejo_root_url: "https://git.example.com/"
```

The Forgejo role writes a Caddy route to `/etc/pp/proxy/routes/forgejo.caddy` by default and proxies to the Forgejo HTTP listener on localhost. The legacy Coolify/Traefik path is still available only when `forgejo_proxy_dynamic_dir` and `forgejo_proxy_network` are explicitly set.

### `pp` Host Ansible Path

The new `pp` deployment system still needs hosts prepared by Ansible, but Ansible should only prepare durable host capabilities:

- admin user and SSH access
- Docker and Docker Compose
- firewall ports
- mounted persistent storage
- optional host-level reverse proxy
- later, any `pp` host-agent installation

Ansible should not deploy individual side-project releases. It should make the host capable of accepting releases from `pp`.

## `pp`: Application Release Deployment

`pp` is the Go deployment CLI in this repo. It is intended for side-project application deploys after the target hosts already have Docker and SSH access.

Current commands:

```sh
go run ./cmd/deploy --help
go run ./cmd/deploy init
go run ./cmd/deploy plan
go run ./cmd/deploy deploy
go run ./cmd/deploy status
go run ./cmd/deploy rollback
go run ./cmd/deploy down
```

See [`examples/`](examples/) for simple website and stateful Go configurations, plus a more complex setup for running Convex in self-hosted mode.

The normal path is config-driven:

- each project has a committed `deploy.yml`
- local `.env` supplies deploy-time secret values
- `pp deploy` builds images locally
- release images and rendered compose bundles are transferred over SSH
- each host runs `docker load` and `docker compose up -d`
- deployment metadata is recorded for status and rollback

`pp` owns application release state such as `.deploy/<project>/<environment>/releases/<release-id>/` locally and the uploaded release bundle on each target host. Use `pp deploy --config deploy.dev.yml` to select an environment's config. Existing state [upgrades automatically](deploy-cli.md#migrating-an-existing-deployment), preserving resource names and rollback history. It should not mutate base OS settings, install Forgejo, or manage platform services.

Published ports default to `127.0.0.1`, including fixed port numbers. Host-level Caddy serves public HTTPS; direct public access requires an explicit `host_ip`. See [current CLI behavior](deploy-cli.md) before upgrading an existing deployment.

## Practical Split

Use Ansible when changing server/platform shape:

- add a new production host
- rotate SSH admin keys
- install or update Docker
- mount a new `/data` volume
- open firewall ports
- install or maintain Forgejo
- install shared reverse proxy or future `pp` host-agent pieces

Use `pp` when changing an application release:

- deploy a side-project container
- run a project migration as part of release flow
- update an app's compose service definition from `deploy.yml`
- check deployment status
- roll back a previous app release

If a change is required for every future deployment on a host, it probably belongs in Ansible. If a change is tied to one project release, it belongs in that project's `deploy.yml` and is applied by `pp`.

## Legacy Coolify Note

Coolify was used previously as a platform layer. New work should not depend on it. Existing Ansible roles may still contain Coolify support for historical compatibility, but the intended split going forward is:

- Ansible for base host and platform services such as Forgejo.
- `pp` for side-project application deployments.
