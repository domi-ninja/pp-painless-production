# Deploy CLI

Go implementation of the side-project deployment system.

## Current Slice

- `go run ./cmd/deploy --help`
- `go run ./cmd/deploy init`
- `go run ./cmd/deploy plan`
- `go run ./cmd/deploy deploy`
- `go run ./cmd/deploy status`
- `go run ./cmd/deploy rollback`
- `go run ./cmd/deploy down`
- Installed locally as `pp`.

`deploy plan` loads `deploy.yml`, validates schema references and required env values, reads git metadata, computes a timestamp-plus-SHA release ID, and prints host/service placement.

`deploy` builds the configured Docker image locally, exports it to `.deploy/<project>/<environment>/releases/<release>/images/`, renders per-host compose bundles, transfers bundles and image tar files over SSH, runs `docker load`, then runs `docker compose up -d` on each host. After a successful deployment, it prints the resolved IP addresses for each deployment domain.

Use `published: auto` for route-backed services. `pp` allocates a stable localhost backend port from `18000-19999`, stores it on the host under `/etc/pp/ports.tsv`, and renders Caddy routes to the allocated port.

Both fixed and automatic ports bind to `127.0.0.1` unless `host_ip` is explicit. Host-level Caddy can reach these ports while serving public HTTPS. To expose a container directly, set `host_ip: 0.0.0.0` or a specific host address. Existing fixed ports without `host_ip` become loopback-only on their next deployment. A proxy on another host or in a separate container network needs an explicitly reachable address.

`status` reads local deployment metadata. `rollback` re-applies the previous local release bundle and runs a configured DB rollback command when migrations are configured.

## Environment isolation

Each deployment belongs to a project and environment. Commands accept `--config` after the command name; paths inside the config remain relative to the working directory.

```sh
pp deploy --config deploy.dev.yml
pp status --config deploy.prod.yml
pp rollback --config deploy.dev.yml
pp down --config deploy.dev.yml
```

For project `shop`, environment `dev`:

| Resource | Identity or location |
| --- | --- |
| Compose containers, networks and managed volumes | Compose project `shop_dev` |
| Local state and releases | `.deploy/shop/dev/` |
| Remote releases | `.pp/shop/dev/releases/` |
| Automatic port key | `shop_dev:<service>:<target>` |
| Caddy route file | `/etc/pp/proxy/routes/shop_dev.caddy` |

`down` requires both `pp.project` and `pp.environment` labels. State and rollback records must match the selected deployment. `plan` prints its identity.

Explicit bind paths, external volumes, fixed ports, image tags, domains and credentials remain your configuration's responsibility. Give them separate values where sharing would be unsafe. Hooks and helper scripts must also select containers using both labels. The CLI cannot isolate arbitrary shell commands.

## Migrating an existing deployment

The naming change requires a planned cutover. The CLI refuses legacy local state, old Compose containers or an old project-only route file before making deployment changes. Legacy Compose volumes also block deployment until their migration is acknowledged. Checks run on every configured host before builds, migrations or port allocation.

1. Back up persistent data and keep the old CLI and release bundles for recovery. Inventory the old containers, volume names, routes and allocated ports.
2. Prepare separate environment configs and update helper scripts. Decide how each existing volume maps to the new deployment. Copy data into the new managed volume or explicitly reference an existing volume as external; changing a Compose name does not migrate data.
3. During the cutover, stop and remove only the old deployment's containers, preserving volumes. Archive its project-only Caddy route outside the imported route directory and reload Caddy. Avoid running old and new containers against the same writable data.
4. Archive the old local `.deploy/state.json` and release bundles outside `.deploy/`. Do not copy old metadata into the new state directory. The first new deployment starts a fresh rollback history.
5. If retaining old Compose volumes, create `.pp/<project>/.environment-isolation-migrated` on each affected host after checking their mappings. This acknowledges retained volumes; it does not bypass checks for old containers or routes.
6. Deploy the selected environment, verify its data and routes, then deploy the other environment with separate storage and credentials. Automatic ports receive new keys and may change. Keep backups until verification passes.

No data migration or legacy-resource deletion happens automatically.
