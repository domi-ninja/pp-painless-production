# Deploy CLI

Go implementation of the side-project deployment system.

New bundles require Docker Compose 2.30+ with raw env-file support. Compose values escape literal dollars and service env files preserve literal values. Old rollback bundles are not rewritten; regenerate a release to apply these protections.

## Current Slice

- `go run ./cmd/deploy --help`
- `go run ./cmd/deploy init`
- `go run ./cmd/deploy plan`
- `go run ./cmd/deploy deploy`
- `go run ./cmd/deploy status`
- `go run ./cmd/deploy rollback`
- `go run ./cmd/deploy down`
- `go run ./cmd/deploy cleanup --dry-run`
- Installed locally as `pp`.

`deploy plan` loads `deploy.yml`, validates schema references and required env values, reads git metadata, computes a timestamp-plus-SHA release ID, and prints host/service placement.

`deploy` builds images locally with a dedicated Buildx builder, exports them to `.deploy/<project>/<environment>/releases/<release>/images/`, renders per-host Compose bundles, transfers only the images each host uses over SSH, runs `docker load`, then runs `docker compose up -d`. Uploaded image tarballs are removed after loading and pinning. After a successful deployment, it prints the resolved IP addresses for each deployment domain.

For upstream images, `pull: if_missing` checks the host's image cache before pulling, including `:latest`. A private pp cache preserves this behavior after pp removes a source tag it introduced. `pull: always` refreshes the image; `pull: never` or an omitted policy requires an image already loaded on the host. Compose startup never pulls a second time. New releases record exact image IDs and use release-specific `pp.local/` tags, so rollback does not resolve mutable upstream tags again. Built images can be restored from local archives; upstream images must remain on the host.

Use `published: auto` for route-backed services. `pp` allocates a stable localhost backend port from `18000-19999`, stores it on the host under `/etc/pp/ports.tsv`, and renders Caddy routes to the allocated port.

Both fixed and automatic ports bind to `127.0.0.1` unless `host_ip` is explicit. Host-level Caddy can reach these ports while serving public HTTPS. To expose a container directly, set `host_ip: 0.0.0.0` or a specific host address. Existing fixed ports without `host_ip` become loopback-only on their next deployment. A proxy on another host or in a separate container network needs an explicitly reachable address.

`status` reads local deployment metadata. `rollback` re-applies the previous local release bundle and runs a configured DB rollback command when migrations are configured.

## Environment isolation

Service `env.required` lists the keys injected from `env.source`, as well as validating their presence. Declare every key the service needs. A source without a key list is rejected unless you explicitly set `env.include_all: true`. This restriction applies to service env files, not build or operator-run hook inputs.

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

## Retention and cleanup

Cleanup runs before a deployment and again when it finishes, including failed attempts. Defaults are finite per project/environment:

```yaml
retention:
  releases: 3          # Successful releases total, including current and previous
  failed: 1           # Failed or interrupted attempts
  min_free_mb: 1024    # Free-space reserve, MiB
  build_cache_mb: 10240 # Dedicated builder cache target, MiB
```

Omitted or zero values use these defaults. At least two successful releases and one failed attempt must be retained. Current and previous releases are always protected, including after rollback.

```sh
pp cleanup --dry-run
pp cleanup
pp cleanup --config deploy.dev.yml
```

Cleanup removes expired local and remote bundles, including old secrets, and tracked Docker image references. It never forces image removal or prunes volumes, containers, unrelated images or the default build cache. Legacy Docker images without ownership records stay untouched; legacy bundle directories can expire normally. Dry-run lists candidates without contacting hosts or deleting artifacts, though loading old state can migrate local metadata.

Local migration images get their own release aliases and recorded IDs. Rollback uses the recorded migration image rather than guessing from the application's first build. Migration image caches follow the same release retention; existing operator-owned source tags remain untouched.

An offline host, an active or stopped container using an expired release, or a cleanup error can temporarily exceed retention. The versioned `cleanup.json` journal remembers pending hosts, including hosts removed from the config. Other hosts still get cleaned. Affected local bundles remain available for recovery until remote cleanup succeeds. Retry with `pp cleanup`; it does not need Git history, build files or secrets. Automatic cleanup errors are reported without undoing a successful deployment.

Use one persistent checkout per deployment in CI, and the same SSH account for operations on a host. Keep `.deploy/` between jobs. Checkout locks and SSH-held host locks prevent overlapping operations; losing a host lock cancels ongoing commands. These locks do not coordinate manual Docker commands or other SSH accounts.

Builds use a pp-owned `docker-container` Buildx builder shared by the local user. pp prunes its cache before and after builds, including build failures; `pp cleanup` also trims an existing pp builder. Your Buildx must support `prune --max-used-space`, `--reserved-space` and `--min-free-space`. The first build downloads BuildKit; base images must be accessible to that builder, not only present in the default Docker image store. Existing default-builder cache is not adopted or pruned.

Release counts are not disk quotas. pp checks local free space before building and exporting, and host bundle/Docker storage before uploading, allowing room for incoming archives. On Docker Desktop, the local daemon's storage is inside its VM; the dedicated builder's pruning policy handles that cache. Builds can still outgrow available space, and protected images, application data, backups and logs need their own disk policies.

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

### Moving a checkout

You can rename or move an app directory together with its `.deploy` directory.
PP resolves recorded bundle and image paths under the current checkout when loading
releases, including older deployment layouts. Project, environment, release ID and
remote resource identities must still match; image artifacts must stay inside their
recorded bundle. Current and previous releases remain available for rollback.
No repair command or manual metadata edit is needed. Loading current-version metadata
does not rewrite it; the next save records the current location.
