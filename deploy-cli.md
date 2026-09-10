# Deploy CLI

Go implementation of the side-project deployment system.

## Current Slice

- `go run ./cmd/deploy --help`
- `go run ./cmd/deploy init`
- `go run ./cmd/deploy plan`
- `go run ./cmd/deploy deploy`
- `go run ./cmd/deploy status`
- `go run ./cmd/deploy rollback`
- Installed locally as `pp`.

`deploy plan` loads `deploy.yml`, validates schema references and required env values, reads git metadata, computes a timestamp-plus-SHA release ID, and prints host/service placement.

`deploy` builds the configured Docker image locally, exports it to `.deploy/releases/<release>/images/`, renders per-host compose bundles, transfers bundles and image tar files over SSH, runs `docker load`, then runs `docker compose up -d` on each host. After a successful deployment, it prints the resolved IP addresses for each deployment domain.

Use `published: auto` for route-backed services. `pp` allocates a stable localhost backend port from `18000-19999`, stores it on the host under `/etc/pp/ports.tsv`, and renders Caddy routes to the allocated port.

`status` reads local deployment metadata. `rollback` re-applies the previous local release bundle and runs a configured DB rollback command when migrations are configured.
