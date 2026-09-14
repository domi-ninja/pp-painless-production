# Humanist Self-Hosted Convex Dev/Prod Isolation Plan

Status: historical proposal, not executed. As of 2026-09-10, the CLI supports separate configs and project/environment isolation. Prefer independent dev and prod deployments using `--config`; the shared-deployment design below needs revision before implementation. See [migration instructions](deploy-cli.md#migrating-an-existing-deployment).

## Decision

Run two independent self-hosted Convex deployments on p3:

- production at `api.humanist.design` with its dashboard at `dash.humanist.design`
- development at `dev-api.humanist.design` with its dashboard at `dev-dash.humanist.design`

The Humanist frontend and `convex dev` process still run on the developer's
machine. They use `.env.local` and connect to the remote development Convex
deployment. Production builds and deployment hooks use `.env.prod` exclusively.

Do not try to represent dev and prod with two admin keys for one Convex backend.
One open-source self-hosted backend is one deployment: one function set, one data
set, and one configured storage layer. Convex Cloud's project-level dev/prod
deployment model is not implemented as namespaces inside a self-hosted backend.

## Verified Convex Behavior

The official self-hosting guide configures a project with one
`CONVEX_SELF_HOSTED_URL` and one admin key for one backend instance. It does not
provide a project/deployment selector within that backend:

- <https://github.com/get-convex/convex-backend/blob/main/self-hosted/README.md>
- <https://docs.convex.dev/self-hosting>

Convex's documented dev/prod/preview deployment selection belongs to Convex
Cloud. The separate local-deployment feature runs another backend process and
stores its state under `.convex`; it does not create a dev namespace inside a
remote self-hosted backend:

- <https://docs.convex.dev/production/overview>
- <https://docs.convex.dev/cli/local-deployments>

The local-deployment feature is a valid alternative for offline/local-only work,
but it has no stable public URL. Humanist benefits from a stable URL for auth
callbacks, HTTP actions, and testing from more than one machine, so a second
remote self-hosted deployment is the recommended choice here.

Admin-key generation is credential issuance, not rotation. The upstream script
issues a new randomly encrypted key from `INSTANCE_NAME` and `INSTANCE_SECRET`.
The backend accepts any key it can decrypt for the matching instance name; the
validation path has no revocation list or admin-key expiry check:

- <https://github.com/get-convex/convex-backend/blob/main/self-hosted/docker-build/generate_admin_key.sh>
- <https://github.com/get-convex/convex-backend/blob/main/crates/keybroker/src/bin/generate_key.rs>
- <https://github.com/get-convex/convex-backend/blob/main/crates/keybroker/src/broker.rs#L927-L1019>

Changing `INSTANCE_SECRET` invalidates previously issued admin keys for that
instance. Generating another key with the same secret does not invalidate older
keys. A `dev:` string added by a dashboard or CLI is only display/type metadata;
the real isolation boundary is a separate backend with a separate instance name
and secret.

## Target Runtime

Keep the existing production services and add four development services to the
same `pp` project:

| Environment | Service | Public endpoint | Persistent state |
| --- | --- | --- | --- |
| prod | `postgres` | none | `/data/pp/humanist-design/postgres` |
| prod | `s3` | SSH-forwarded console only | `/data/pp/humanist-design/s3` |
| prod | `convex-backend` | `api.humanist.design` | prod Postgres and prod S3 |
| prod | `convex-dashboard` | `dash.humanist.design` | none |
| dev | `dev-postgres` | none | `/data/pp/humanist-design/dev-postgres` |
| dev | `dev-s3` | SSH-forwarded console only | `/data/pp/humanist-design/dev-s3` |
| dev | `dev-convex-backend` | `dev-api.humanist.design` | dev Postgres and dev S3 |
| dev | `dev-convex-dashboard` | `dev-dash.humanist.design` | none |

Four services, rather than only another backend and dashboard, are recommended
because they make the data boundary structural. Sharing the production Postgres
and MinIO engines would require database bootstrap between `pp`'s infra and
backend phases, scoped MinIO users/policies, and careful protection against
cross-environment credentials. Current `pp` has no hook between those phases.
Separate stateful containers fit the existing phase model and are harder to
misconfigure.

No development frontend container is needed. Vite remains local on port `3333`.

## Environment Contract

`.env.local` must be rebuilt as a dev-only file; do not edit the current file in
place or carry production values forward. It should contain:

- `CONVEX_SELF_HOSTED_URL=https://dev-api.humanist.design`
- `VITE_CONVEX_URL=https://dev-api.humanist.design`
- `VITE_CONVEX_SITE_URL=https://dev-api.humanist.design`
- `VITE_SITE_URL=http://localhost:3333`
- `SITE_URL=http://localhost:3333`
- `INSTANCE_NAME=dev-api.humanist.design`
- a unique dev `INSTANCE_SECRET`
- a dev admin key issued by `dev-convex-backend`
- unique dev Postgres and MinIO credentials
- `humanist-dev-convex-*` bucket names

`.env.prod` remains the only production source and should contain:

- `CONVEX_SELF_HOSTED_URL=https://api.humanist.design`
- production frontend/site URLs
- `INSTANCE_NAME=api.humanist.design`
- the production instance secret and admin key
- production-only Postgres and MinIO credentials
- `humanist-prod-convex-*` bucket names

The current unsuffixed production buckets need an explicit migration decision.
Renaming an env variable does not move existing objects. Keep the existing
production bucket names during the first isolation cutover unless their contents
are copied and verified separately.

Do not push deployment/runtime variables wholesale into Convex function env.
Change `scripts/push-env-to-convex.sh` to use an allowlist of function-facing
variables. It must exclude admin keys, instance secrets, Postgres credentials,
MinIO root credentials, and Convex backend storage configuration.

## Humanist Changes

### `deploy.yml`

1. Add `dev-postgres` and `dev-s3` in the `infra` phase.
2. Add `dev-convex-backend` and `dev-convex-dashboard` in the `backend` phase.
3. Make every dev service read `.env.local`; existing production services keep
   reading `.env.prod`.
4. Configure `dev-convex-backend` with:
   - origins set to `https://dev-api.humanist.design`
   - `POSTGRES_URL` targeting `dev-postgres`
   - `S3_ENDPOINT_URL=http://dev-s3:9000`
   - only dev credentials and `humanist-dev-*` buckets
5. Configure the dev dashboard to target the dev backend only.
6. Publish independent auto-allocated host ports for both dev backend ports and
   the dev dashboard.
7. Keep the production Vite build arguments sourced only from `.env.prod`.

### Caddy and DNS

1. Add DNS records for `dev-api.humanist.design` and
   `dev-dash.humanist.design` pointing to p3.
2. Extend `deploy/caddy/humanist.caddy.tmpl` with a second copy of the Convex API
   routing rules targeting `dev-convex-backend`.
3. Route `dev-dash.humanist.design` only to `dev-convex-dashboard`.
4. Restrict the dev API's CORS origins to the actual local frontend origins used
   by Humanist, initially `http://localhost:3333` and
   `http://127.0.0.1:3333`.
5. Do not expose either MinIO console through Caddy. Extend the SSH-forwarding
   script with `--dev` and `--prod` service selection.

### Local commands and safety rails

1. Change `dev:backend` to pass `.env.local` explicitly.
2. Add a preflight command before both Vite and `convex dev` that rejects:
   - any local Convex URL equal to `api.humanist.design`
   - any local site URL equal to `humanist.design`
   - missing or non-`humanist-dev-*` bucket names
   - a key whose embedded instance name is not `dev-api.humanist.design`
3. Make production sync commands pass `.env.prod` explicitly and assert the
   inverse hostnames.
4. Remove `update-browserslist-db` from `pnpm dev`; it mutates the lockfile as a
   side effect of starting development.
5. Add a tiny diagnostic command that prints only selected environment names and
   hostnames, never credentials, so target selection is visible before sync.

## Admin-Key Handling

Replace the current refresh semantics with ensure semantics:

1. Rename the script to `scripts/ensure-convex-admin-key.sh`.
2. Default to dev: `.env.local`, expected instance
   `dev-api.humanist.design`, service `dev-convex-backend`.
3. `--prod` selects `.env.prod`, expected instance `api.humanist.design`, and
   service `convex-backend`.
4. If a key is present, verify it against the selected URL and leave it alone.
5. If it is absent, generate one from the selected container, verify the key's
   embedded instance name, test a harmless CLI operation, and persist it.
6. Remove `--force` from normal deploy hooks. Reissuing keys creates additional
   valid credentials and is not rotation.
7. Add a separate, deliberately named `rotate-convex-instance-secret.sh` runbook
   for actual revocation. It must back up the database, replace the selected
   instance secret, restart only that backend, generate a new admin key, verify
   the deployment, and record expected effects on sessions/scheduled work.

The production secret should not be rotated automatically during this cutover.
First test secret rotation against the new dev instance. The old production key
currently present in `.env.local` should be removed immediately when that file is
rebuilt; rotate the production instance secret afterward only if invalidating all
previously generated production keys is required.

## Deployment Sequence

1. Stop the current local `pnpm dev`; it is attached to production.
2. Back up production Postgres and inventory the current production MinIO
   buckets before changing runtime configuration.
3. Create DNS records for the two dev hostnames.
4. Build a new `.env.local` from dev values and unique generated secrets.
5. Add the four dev services and Caddy routes to `deploy.yml`.
6. Run `pp plan` and verify every dev service reports `.env.local` while every
   prod service/build/hook reports `.env.prod`.
7. Run `pp deploy`. The infra phase starts both dev state stores; the backend
   phase starts both dev Convex containers; Caddy then exposes the dev routes.
8. Ensure the dev MinIO buckets and dev admin key.
9. Push the allowlisted dev function env.
10. Run `pnpm dev`; its first `convex dev` sync initializes dev functions/schema.
11. Seed or import only intentionally selected development data.
12. Run isolation tests before resuming normal development.

Production deploy hooks should continue syncing production functions from
`.env.prod`. They should provision and health-check the dev services but should
not run a one-shot dev function sync on every production deploy; the local
`convex dev` process owns dev code synchronization.

## Acceptance Tests

- `pnpm dev` preflight prints `dev-api.humanist.design` and refuses to start if
  `.env.local` contains a production hostname.
- Browser-loaded Vite modules contain only the dev Convex URL.
- The local `convex dev` process connects only to the dev API IP/hostname.
- `https://dev-api.humanist.design/version` and
  `https://dev-dash.humanist.design/` pass smoke checks.
- Creating a sentinel document through dev does not make it visible through a
  production query; the inverse test also passes.
- Uploading a dev file creates objects only under the dev MinIO data path and dev
  bucket names.
- A dev admin key fails against the production backend, and a production admin
  key fails against the dev backend.
- Restarting or deleting dev containers does not stop production containers or
  alter production data.
- Production frontend, HTTP action, Convex query, dashboard, and Playwright smoke
  checks still pass after the dev stack is added.
- Secret values do not appear in `pp plan`, hook output, generated metadata, or
  git diffs.

## Rollback

The first rollout is additive. If dev provisioning fails:

1. Remove the two dev Caddy routes.
2. Remove only the four `dev-*` containers.
3. Preserve `/data/pp/humanist-design/dev-postgres` and
   `/data/pp/humanist-design/dev-s3` until the failure is understood.
4. Leave all existing production services, routes, bind mounts, env values, and
   production bucket names unchanged.

The legacy `pp down` removes every container with `pp.project=humanist-design`, so it
is not an environment-selective rollback command. Use explicit service-scoped
cleanup during this rollout.

## Open Questions

1. Should dev and prod remain in one `pp` project as requested, accepting that
   `pp down`, status, and rollback operate on both, or should `pp` first gain a
   `--config` option plus project-scoped local state so dev and prod can be
   independent `humanist-design-dev` and `humanist-design-prod` projects?
   Independent projects are the safer long-term model.
2. Does dev need inbound third-party webhooks/OAuth callbacks? If not, Convex's
   official local-deployment mode is simpler and removes all four remote dev
   services.
3. Should `dev-dash.humanist.design` have an additional Caddy authentication
   layer, or is Convex dashboard key authentication sufficient?
4. Should existing production objects stay in the unsuffixed buckets, or should
   a separately verified copy/migration move them to `humanist-prod-*`?
5. After testing rotation on dev, should the production `INSTANCE_SECRET` be
   rotated once to invalidate every production admin key generated during the
   earlier broken setup?
