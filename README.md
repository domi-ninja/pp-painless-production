# Painless Production

`pp` deploys your applications to Docker hosts over SSH. Run it from your app's repository: it builds images locally, uploads release bundles, starts services, updates Caddy routes and runs your deployment checks.

```sh
pp plan     # Inspect the selected deployment
pp          # Build and deploy
pp status   # Read the recorded release state
pp rollback # Reapply the previous release
```

## Start here

This is an experimental tool meant to be used from a full clone of this repository. Keep the source, examples and Ansible playbook together, and build the CLI locally with `make install`. Expect to inspect and adapt the configuration for your hosts. This is not a standalone binary installer or a managed hosting service.

Follow [Getting started](GETTING_STARTED.md) to clone the repo, install `pp`, prepare a server if needed, and deploy your first app.

## Everyday deployment

Each app has its own committed `deploy.yml`. It describes the images, target hosts, ports, persistent storage, routes and optional hooks or checks. Secrets stay in ignored env files.

`pp` builds and exports images on your machine, then transfers them over SSH. Hosts load the images and run Docker Compose. Published ports default to loopback; host-level Caddy serves public HTTPS.

Project and environment identify each deployment. Use `pp deploy --config deploy.dev.yml` to select a different config. Local state and release metadata live under `.deploy/<project>/<environment>/`; older state upgrades automatically while preserving existing resource names and rollback history.

- [CLI behavior and migrations](deploy-cli.md)
- [Example applications](examples/)
- [Deployment security review](security-review-deployment.md)

## Server provisioning, occasionally

Ansible is a setup subtask, not part of every deployment. Use the bundled [provisioning playbook](infra/ansible/README.md) when adding a host or deliberately changing its system configuration. It prepares Ubuntu, SSH access, Docker, firewall rules and optional Caddy or storage mounts. Skip provisioning when your host already meets the requirements.

After setup, return to your app repository and use `pp`. You do not need to rerun Ansible to ship an application change. Forgejo hosting and CI runner provisioning live separately in [git-host-ansible](https://git.domi.ninja/domi-ninja/git-host-ansible).

## Source layout

```text
cmd/deploy/       CLI entrypoint, built and installed as pp
internal/deploy/  Deployment implementation
examples/         Application deployment configs
infra/ansible/    Occasional server provisioning
tickets/          Implementation work
```

The `deployment-*.md` files contain design notes; [deploy-cli.md](deploy-cli.md) describes current behavior.

## What does `pp` stand for?

- **Painless Production.** The official name and the promise we're trying to keep.
- **Push & Pray.** Historically accurate.
- **Predictable Production.** The engineering goal.
- **Pocket Platform.** Small deployment tool, no platform circus.
