# Commands and deployment behavior

Run pp from the application's working directory. It loads `deploy.yml` unless you pass `--config` after the command. Config paths remain relative to that working directory.

| Command | Behavior |
| --- | --- |
| `pp init` | Create a starter config and ignore rules; refuse an existing config. |
| `pp plan` | Validate config and inputs; preview placement, images, env key scopes, mounts, routes and recovery policy. |
| `pp deploy` or `pp` | Build and apply a release, then run checks. |
| `pp status` | Print local release and attempt records, then inspect recorded hosts. |
| `pp rollback` | Recover the previous release using saved inputs. Read [recovery](recovery.md) first. |
| `pp down` | Stop owned containers and remove owned routes; preserve data and port reservations. |
| `pp down --release-ports` | Also retire the deployment's reserved ports. |
| `pp cleanup --dry-run` | Preview expired bundles and owned image references. |
| `pp cleanup` | Retry pending cleanup and enforce retention where possible. |
| `pp version` | Print the compiler, source revision and modified-source status. |
| `pp help COMMAND` | Print a command's options and description. |

```sh
pp plan --config deploy.dev.yml
pp deploy --config deploy.prod.yml
pp status --config deploy.prod.yml
```

Plan does not contact hosts or reserve automatic ports. It can upgrade old local state. It prints env key names, never values. Deployment validates local and remote tools, paths, free space and host architecture before building or migrating. An upstream-only deployment needs no local Docker daemon unless it runs a migration.

## Applying a release

pp builds with its own Docker-container Buildx builder. The first build downloads BuildKit. It renders private per-host Compose bundles and selected runtime env files, exports built images, uploads only the images each host needs and loads them. Uploaded archives are removed after loading and pinning.

Upstream `pull: if_missing` reuses the host cache, including pp's private cache for images it first pulled. `always` refreshes the source image; `never` and omission require an image already present. pp records exact image IDs and uses release-specific aliases. Compose startup does not pull again. Built images can be restored from local archives; upstream images must remain on the host.

A declared migration runs locally before service apply. Phased apply starts infrastructure and waits, runs `after_infra_healthy`, starts backends and waits, applies routes, runs `after_backend_healthy`, then applies all services and runs `after_services_healthy`. The final apply removes retired services and waits up to 300 seconds. Unphased apply also waits. Health checks must measure the app's readiness.

Routes are applied again before smoke checks. HTTP checks retry within a bounded timeout; command checks run once. pp records host apply progress and check results even when a later host fails. See [configuration](configuration.md) for timeout defaults and [recovery](recovery.md) for failure behavior.

## Identity and isolation

Each deployment belongs to a project and environment. For a new `shop` deployment in `dev`:

| Resource | Identity or location |
| --- | --- |
| Compose project | `shop_dev` |
| Local state and releases | `.deploy/shop/dev/` |
| Remote releases | `.pp/shop/dev/releases/` |
| Automatic port key | `shop_dev:SERVICE:TARGET` |
| Caddy route file | `/etc/pp/proxy/routes/shop_dev.caddy` |

Down selects both `pp.project` and `pp.environment` labels. State records must match the selected identity. Explicit bind paths, external volumes, fixed ports, image tags, domains and credentials still need separate values where sharing would be unsafe. Hooks are trusted scripts; labels cannot constrain arbitrary shell commands.

Use a persistent checkout per deployment and the same SSH account on a host. Keep `.deploy/` between CI jobs. Checkout and SSH-held host locks prevent overlapping pp operations; losing a host lock cancels work. Manual Docker commands and other SSH accounts are outside those locks.

## Ports and routes

Ports default to `127.0.0.1`. Automatic ports come from 18000 through 19999 and remain stable in `/etc/pp/ports.tsv`. Allocation considers saved reservations, listening sockets, Docker bindings and fixed ports in the pending plan. Corrupt or conflicting records fail; pp does not silently reassign them.

Use `${service.NAME.port.TARGET}` in a route template for a fixed or automatic port. Set `host_ip` explicitly for a different binding. A public Docker port needs a Docker-aware or upstream firewall; UFW alone does not restrict it.

pp stages and validates the complete shared Caddy configuration before replacing its own route file. Failed reload restores that file and reloads the old configuration. Keep `import /etc/pp/proxy/routes/*.caddy` on its own line and use absolute paths for other imports. Custom templates are trusted Caddy configuration.

Removing a route updates the deployment's route file. Removing services from the final Compose apply preserves their data volumes. Removing a host that still has owned containers fails before building or migrating. Down reads saved hosts so you can retire the old deployment even after editing today's config.

Down keeps reserved ports unless `--release-ports` is explicit. Retention never reclaims them. Recovery after releasing ports can fail if another deployment has taken them.

## Status

Status needs deployment identity and local records, not current env files, build inputs or Git history. It prints the current and previous successful releases separately from the latest deploy or rollback attempt. It then inspects hosts with a 15 second limit per host, reporting container release labels, state, health and drift. An unreachable host returns failure.

A successful local record is not proof of the current host state. Compare the observed release and expected services, especially after manual host changes or interrupted deployment.

## Retention and cleanup

Defaults are finite per project/environment:

```yaml
retention:
  releases: 3
  failed: 1
  min_free_mb: 1024
  build_cache_mb: 10240
```

Zero uses the default. At least two successful releases and one failed attempt must remain. Current and previous releases are always protected. Cleanup runs before and after deployment, including failures.

Cleanup removes expired private bundles, old secrets and recorded image references. It does not force image removal or prune volumes, containers, unrelated images or the default build cache. Images without ownership records remain untouched. Migration images have their own recorded IDs and release aliases.

Offline hosts, containers still using an expired release and cleanup errors can exceed the configured retention. A versioned `cleanup.json` journal tracks pending hosts, including removed hosts. Their local bundles remain until remote cleanup succeeds. Retry with `pp cleanup`; it does not need build files or secrets. A cleanup error does not undo a successful deployment.

Dry-run lists candidates without contacting hosts or deleting artifacts, though loading old state can upgrade metadata. The dedicated builder is pruned before and after builds and by cleanup. Buildx must support `prune --max-used-space`, `--reserved-space` and `--min-free-space`. Existing default-builder cache remains separate.

Release counts are not disk quotas. Free-space checks allow room for incoming archives, but builds can still outgrow storage. Protected images, application data, backups and logs need separate disk policies. Docker Desktop stores its daemon data inside a VM.

## Migrating an existing deployment

Config version remains 1. State and release metadata use version 2; unversioned metadata is v1. Unknown newer formats fail and require a newer pp. The CLI does not rewrite config YAML.

Legacy `.deploy/state.json` identifies the owning project and environment. pp copies its release metadata into the scoped state directory and preserves current/previous IDs, Compose name, managed volumes, port keys, route filename and remote paths through a stable `resource_id`. Original state and bundles remain. Imported records can reference `.deploy/releases/`; do not delete it. Scoped v1 upgrades retain `.v1.bak` copies.

The host ownership check records a versioned claim at `.pp/PROJECT/resource-owner` for adopted resources. Run the original environment once before adding another environment beside legacy resources. Conflicting ownership, missing releases or mixed container layouts require investigation; pp will not guess or initialize a replacement database.

You can move a checkout together with `.deploy/`. Saved paths resolve beneath the new checkout, while identity and artifacts must still match. Loading current-version metadata does not rewrite it; the next save records the new location.

## Build identity

App builds use the working tree. The dirty digest describes Git status and the tracked diff. It does not hash untracked contents or prove that the complete Docker context matches a commit. Review untracked inputs and `.dockerignore` before deployment. Keep secrets and `.deploy` out of build contexts.
