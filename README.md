# Painless production

pp builds your app's Docker images and deploys them to hosts over SSH. It keeps release bundles, waits for service health, updates Caddy routes and runs deployment checks. It is an experimental tool for trusted operators.

## First deployment

Install from this repository with a supported, patched Go compiler, starting with Go 1.26.9:

```sh
git clone https://github.com/domi-ninja/pp-painless-production.git
cd pp-painless-production
make install
pp version
```

Add `$(go env GOPATH)/bin` to your PATH if needed. Prepare a host with Docker, Compose 2.30+, SSH and optional Caddy using [Getting started](docs/GETTING_STARTED.md). Local image builds also require Docker and Buildx on your deployment machine.

Then work from your app's Git repository, which must already have a Dockerfile:

```sh
cd /path/to/your-app
pp init
# Edit deploy.yml with your host, domain, container port and health endpoint.
pp plan
pp deploy
pp status
```

`init` refuses to overwrite a config and adds ignore rules for state and env files. Keep secrets out of Git and the build context. Preserve `.deploy/`; it contains release history, old runtime secrets and recovery artifacts. Back up application data separately.

Use [deployment examples](examples/README.md) for a static website, a Go app with SQLite or a self-hosted Convex app. These are templates to copy into your app, not standalone applications.

## Guides

- [Getting started](docs/GETTING_STARTED.md) covers installation and host setup.
- [Commands and deployment behavior](docs/deploy-cli.md) covers releases, status, ports and retention.
- [Configuration](docs/configuration.md) lists implemented fields and defaults.
- [Recovery](docs/recovery.md) explains rollback limits and saved inputs.
- [Security](docs/security-review-deployment.md) explains credentials and host trust.
- [Development](docs/development.md) covers builds and isolated validation.
