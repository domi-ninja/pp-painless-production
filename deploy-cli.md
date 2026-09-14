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

For upstream images, `pull: if_missing` checks the host's image cache before pulling, including `:latest`. `pull: always` refreshes the image; `pull: never` or an omitted policy requires an image already loaded on the host. Compose startup never pulls a second time. Rollback uses saved bundles and available images without refreshing upstream tags.

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

For a new deployment of project `shop`, environment `dev`:

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

Run `pp` as usual. Local state upgrades automatically before deployment. `pp status` or `pp plan` can perform the local upgrade without changing the running site.

- Config remains version `1`; omitting `version` means `1`. The CLI does not rewrite your YAML.
- State and release metadata use version `2`. Unversioned files are v1. Unknown newer versions fail before deployment and ask you to upgrade `pp`.
- Legacy `.deploy/state.json` identifies the owning project and environment. The CLI copies that environment's release metadata into `.deploy/<project>/<environment>/` and preserves current and previous release IDs.
- Existing deployments retain their Compose name, managed volumes, port allocation keys, route filename and remote release paths. State records this stable `resource_id`. New environments get `<project>_<environment>` resources.
- Original state and bundles stay in place. Imported records still reference the old bundles, so do not delete `.deploy/releases/`. Upgrades of already-scoped v1 files keep `.v1.bak` copies. Writes are atomic; concurrent local migration attempts share a lock.

Before deployment, the CLI checks container ownership on every host. It then records a versioned claim at `.pp/<project>/resource-owner` for adopted resources. A second environment or checkout cannot claim the same resources. Run the original environment once before adding another environment beside legacy resources.

Missing release records, conflicting ownership, or both old and new container layouts require investigation. The CLI will not guess, delete volumes or start a replacement database. A deployment already migrated to scoped resources, such as Humanist production, keeps its current identity.

For future format changes, add an explicit version migration and tests. Reading an old format must not silently rename infrastructure or discard rollback history. Config and state versions advance independently.
