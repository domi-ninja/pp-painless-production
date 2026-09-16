# Humanist Convex/Vite `pp` Deployment Plan

Historical implementation plan. Production migrated to environment-scoped deployment on 2026-09-14. Commands, env files and paths below describe the original design; use [deployment notes](humanist-convex-pp-deployment-notes.md) for current operation.

Goal: create a lightweight Docker deployment process for `../humanist.design` using the `pp` deploy command in this repo. The process should deploy the Vite frontend plus a self-hosted Convex backend/dashboard, Postgres, and S3-compatible object storage, push `.env.local` values into Convex, run the Convex CLI so production functions/schema are current, and apply everything over SSH without introducing a project-owned image registry or heavyweight platform.

Confirmed target: `p2.domi.ninja` is the working reference implementation. The new automated deployment should go to `p3.domi.ninja`, using SSH target `deploy@p3.domi.ninja`.

## Resolved Clarifications

- Deploy to `p3.domi.ninja`; use p2 only as the reference implementation.
- Use `deploy@p3.domi.ninja` for `pp`.
- Expose `dash.humanist.design`; the dashboard has its own auth using the Convex instance master key, which can be regenerated/reset with an existing script.
- Use the p2 dashboard pattern: dashboard listens on container port `3211`, not the self-host example's `6791`.
- Push all of `.env.local` into Convex for now.
- Use `npx convex dev --once --env-file .env.local` as the production sync command unless testing proves otherwise.
- Do not implement Postgres backups in this first pass; track it as a separate ticket.
- Use Convex `latest` for the first pass. There are two version surfaces: the app CLI/package should use `convex@latest` via pnpm, and the self-hosted backend/dashboard containers should use the upstream Convex `latest` images initially.
- Add S3-compatible object storage as a container for Convex file storage.
- Redirect `www.humanist.design` to `humanist.design`.

## Sources Checked

- Current `pp` docs: `deploy-cli.md`
- Current `pp` schema and implementation: `internal/deploy/config.go`, `internal/deploy/operations.go`, `internal/deploy/compose.go`, `internal/deploy/ports.go`
- Current Humanist app repo: `../humanist.design`
- Working p2 Docker runner config: `../docker-runner-p2.domi.ninja/docker-compose.yml`
- Working p2 Caddy routing: `../docker-runner-p2.domi.ninja/config/Caddyfile`
- Local Convex self-host compose example: `../convex-selfhost-dockercompose/docker-compose.yml`

## Current Humanist Shape

`../humanist.design` is a Vite/React/Convex app using pnpm. Important local facts:

- `package.json` has `build`: `tsc -p convex --noEmit --pretty false && tsc -p tsconfig.app.json --noEmit --pretty false && convex dev --once && vite build`.
- Frontend runtime reads `VITE_CONVEX_URL` and `VITE_CONVEX_SITE_URL`.
- `.env.local` includes:
  - `CONVEX_SELF_HOSTED_URL`
  - `CONVEX_SELF_HOSTED_ADMIN_KEY`
  - `VITE_CONVEX_URL`
  - `VITE_CONVEX_SITE_URL`
  - `VITE_SITE_URL`
  - `SITE_URL`
  - Scaleway and auth secrets
- `scripts/push-.env.local-to-convex.sh` already pushes `.env.local` into Convex, but needs hardening before automation.

The p2 reference publishes Humanist with three containers:

- `convex-backend-humanist-design`
  - image `ghcr.io/get-convex/convex-backend:latest`
  - backend API on container port `3210`
  - HTTP actions/site proxy on container port `3211`
  - data volume mounted at `/convex/data`
- `convex-dashboard-humanist-design`
  - image `ghcr.io/get-convex/convex-dashboard:latest`
  - configured with `PORT=3211`
  - proxied at `dash.humanist.design`
- `convex-web-humanist-design`
  - nginx serving `dist/`
  - proxied at `humanist.design` and `www.humanist.design`

The p2 Caddy routing for `api.humanist.design` is important:

- `OPTIONS` requests get a CORS preflight response.
- websocket upgrade traffic goes to backend `3210`.
- `/api/auth/*` and `/.well-known/*` go to backend `3211`.
- everything else goes to backend `3210`.

## Current `pp` Capability

`pp` already does the useful base workflow:

- reads committed `deploy.yml`
- reads local env files
- builds one Docker image locally
- saves that image tar under `.deploy/releases/<release>/images/`
- renders one compose bundle per host
- uploads bundle and image tar over SSH
- runs `docker load`
- runs `docker compose up -d`
- allocates stable localhost ports under `/etc/pp/ports.tsv`
- writes simple Caddy route files under `/etc/pp/proxy/routes/`
- runs one pre-service migration command inside the built image

The Humanist Convex deployment needs small extensions:

- multiple services in one compose project
- services that use upstream images and need `docker compose pull`
- one or more local builds, not exactly one global build
- build args/env for Vite
- Caddy route templates instead of only one-hostname-to-one-target routes
- local deploy hooks after backend health
- an explicit Postgres service or external Postgres configuration
- service ordering that updates Convex before switching frontend code

## Target Architecture

Run one `pp` compose project on `p3.domi.ninja`.

Public hostnames:

- `humanist.design` -> Vite frontend
- `www.humanist.design` -> redirect to `https://humanist.design`
- `api.humanist.design` -> Convex backend API and HTTP actions
- `dash.humanist.design` -> Convex dashboard

Compose services:

- `web`
  - locally built image, for example `humanist-design-web:${release}`
  - nginx or Caddy runtime serving Vite `dist/`
  - depends on the Convex deployment having been synced before traffic switches
- `convex-backend`
  - external image `ghcr.io/get-convex/convex-backend:latest` initially
  - env file rendered by `pp`
  - health check on `http://127.0.0.1:3210/version`
  - exposes `3210` and `3211` to stable host-local ports, or only inside the compose network if Caddy joins it
- `convex-dashboard`
  - external image `ghcr.io/get-convex/convex-dashboard:latest`
  - configured with the same origins as backend
  - expose container port `3211`, matching p2 behavior
  - public at `dash.humanist.design`; dashboard auth is handled by the Convex instance master key
- `postgres`
  - `postgres:16-alpine` or `postgres:17-alpine`
  - persistent data on p3, preferably a bind mount under `/data/pp/humanist-design/postgres`
  - backend receives `POSTGRES_URL=postgres://...@postgres:5432/...`
- `s3`
  - S3-compatible object storage container, likely MinIO unless p3 already has a preferred local S3 service
  - persistent data on p3, preferably a bind mount under `/data/pp/humanist-design/s3`
  - backend receives the Convex `S3_STORAGE_*`, `AWS_*`, and endpoint env vars required by the self-host image

Convex file/object storage is in scope for the first deployment, so the plan should include bucket creation/configuration and the corresponding Convex backend env vars.

## Lightweight Docker Model

Use `pp` as the control plane:

- local machine builds only project-owned images
- host pulls upstream Convex/Postgres/nginx base images
- no image registry in v1
- deployment bundles remain under `.deploy/releases/`
- host release bundles remain under `/etc/pp/releases/<project>/<release>/`
- Caddy stays host-level and imports route fragments from `/etc/pp/proxy/routes`

For p3 persistence:

- Postgres data should not live under a release directory.
- Prefer `/data/pp/humanist-design/postgres:/var/lib/postgresql/data`.
- S3-compatible object storage data should not live under a release directory.
- Prefer `/data/pp/humanist-design/s3:/data`.
- Add a named Docker volume only if p3 backup tooling already captures Docker volumes reliably.
- Do not block first deploy on backups. Track Postgres backup/retention separately in `tickets/humanist-convex/005-add-postgres-backups.md`.

## Required `pp` Changes

### 1. Multi-build and external-image services

Replace the single global `build` assumption with `builds`, while preserving existing `build` for simple projects.

Sketch:

```yaml
builds:
  web:
    context: .
    dockerfile: Dockerfile
    target: runtime
    tags:
      - "humanist-design-web:${release}"
    args:
      VITE_SITE_URL: "${env.VITE_SITE_URL}"
      VITE_CONVEX_URL: "${env.VITE_CONVEX_URL}"
      VITE_CONVEX_SITE_URL: "${env.VITE_CONVEX_SITE_URL}"

services:
  web:
    image: "humanist-design-web:${release}"
    build: web

  convex-backend:
    image: "ghcr.io/get-convex/convex-backend:latest"
    pull: if_missing
```

Implementation detail:

- Build only services that reference a local `build`.
- Save one tar per local build.
- Upload/load all local tar files.
- Before `docker compose up -d`, run `docker compose pull` for services with `pull: if_missing` or `pull: always`.

### 2. Build args from env

Vite values are baked at build time. Add explicit build args sourced from env instead of passing the entire `.env.local` into Docker build.

Minimum:

- load `.env.production` or `.env.local`
- allow `${env.NAME}` interpolation in build args
- validate required build arg names exist
- reject accidental secret-looking build args for frontend builds unless explicitly allowed

Humanist frontend build args should include only public values:

- `VITE_SITE_URL=https://humanist.design`
- `VITE_CONVEX_URL=https://api.humanist.design`
- `VITE_CONVEX_SITE_URL=https://api.humanist.design`

### 3. Rich Caddy route templates

The current structured `routes` block cannot express the p2 Convex routing. Add a lightweight `route_files` feature.

Sketch:

```yaml
route_files:
  - source: deploy/caddy/humanist.caddy.tmpl
    host: p3
    dest_name: humanist-design.caddy
```

Template variables should cover:

- `${project}`
- `${release}`
- `${service.convex-backend.port.3210}`
- `${service.convex-backend.port.3211}`
- `${service.web.port.80}`
- `${env.PUBLIC_HOSTNAME}` for non-secret values

For the first implementation, copy the p2 Caddy structure into `deploy/caddy/humanist.caddy.tmpl` and replace hard-coded service names/ports with template variables.

### 4. Local deploy hooks

Convex needs local CLI operations after the backend is reachable.

Add generic hooks:

```yaml
hooks:
  after_services_healthy:
    - name: push convex env
      run: ["bash", "scripts/push-.env.local-to-convex.sh", "prod"]
      env:
        source: ".env.local"
        required:
          - CONVEX_SELF_HOSTED_URL
          - CONVEX_SELF_HOSTED_ADMIN_KEY

    - name: sync convex functions
      run: ["npx", "convex", "dev", "--once", "--env-file", ".env.local"]
      env:
        source: ".env.local"
        required:
          - CONVEX_SELF_HOSTED_URL
          - CONVEX_SELF_HOSTED_ADMIN_KEY
```

Run these hooks locally from `../humanist.design`, not inside a container. The Convex CLI expects the checked-out repo and `convex/` directory, and local node dependencies already contain the CLI.

### 5. Service groups or phased apply

The release should not switch the frontend before Convex sync succeeds. Keep this simple:

1. Apply infrastructure services first: `postgres`, `s3`, `convex-backend`, `convex-dashboard`.
2. Wait for backend health.
3. Push Convex env.
4. Run `convex dev --once`.
5. Build or start/switch frontend.
6. Reload Caddy route.
7. Run smoke checks.

Possible `deploy.yml` model:

```yaml
services:
  postgres:
    phase: infra
  convex-backend:
    phase: backend
  convex-dashboard:
    phase: backend
  web:
    phase: frontend
```

Implementation can be crude in v1: render the whole compose file, run `docker compose up -d postgres s3 convex-backend convex-dashboard`, run hooks, then run `docker compose up -d`.

### 6. Health checks and smoke checks

Before hooks:

- wait for `convex-backend` Docker health
- confirm `https://api.humanist.design/version` or a p3 host-local equivalent returns `200`

After deploy:

- `https://humanist.design/` returns `200`
- `https://api.humanist.design/version` returns `200`
- `https://dash.humanist.design/` returns `200`, `401`, or another agreed expected auth response
- optionally run one cheap Convex query/action if Humanist has a stable one

## Humanist Repo Changes

Add:

- `Dockerfile` for static Vite build and runtime server
- `nginx.conf` with SPA fallback
- `deploy.yml`
- `deploy/caddy/humanist.caddy.tmpl`
- `.env.production.example` for frontend build-time public values
- `.env.convex.backend.example` for Convex backend service env
- `.env.postgres.example` or documented required Postgres keys
- `.env.s3.example` or documented required local S3 keys/buckets

Fix `scripts/push-.env.local-to-convex.sh` before it becomes part of deploy:

- quote `$1`, `$DEPLOYMENT`, variable names, and values consistently
- pass `$DEPLOYMENT` to `npx convex env list` and `npx convex env remove`, not only `env set`
- do not parse `env list` with unsafe word splitting
- avoid deleting every existing Convex env var by default; prefer upsert only, or require `--prune`
- keep `.env.local` as the source for now, including `VITE_*` values
- optionally allow a safer source file such as `.env.convex.prod` later
- use `--env-file .env.local` with Convex CLI commands so self-hosted URL/admin key selection is explicit

Consider changing `package.json` for deployment so `vite build` and `convex dev --once` can run separately. The current `build` script already runs `convex dev --once`, which is convenient locally but awkward for staged deploy ordering. Add scripts such as:

```json
{
  "scripts": {
    "build:web": "tsc -p tsconfig.app.json --noEmit --pretty false && vite build",
    "sync:convex": "tsc -p convex --noEmit --pretty false && convex dev --once --env-file .env.local"
  }
}
```

## Example `deploy.yml` Sketch

This is illustrative. Exact syntax depends on the `pp` changes above.

```yaml
version: 1

project:
  name: humanist-design
  environment: prod

env:
  source: ".env.local"

builds:
  web:
    context: .
    dockerfile: Dockerfile
    target: runtime
    tags:
      - "humanist-design-web:${release}"
    args:
      VITE_SITE_URL: "${env.VITE_SITE_URL}"
      VITE_CONVEX_URL: "${env.VITE_CONVEX_URL}"
      VITE_CONVEX_SITE_URL: "${env.VITE_CONVEX_SITE_URL}"

hosts:
  p3:
    ssh: deploy@p3.domi.ninja
    roles: [web, convex]

services:
  postgres:
    image: "postgres:16-alpine"
    pull: if_missing
    phase: infra
    hosts: [p3]
    env:
      source: ".env.postgres"
      required:
        - POSTGRES_DB
        - POSTGRES_USER
        - POSTGRES_PASSWORD
    volumes:
      - source: "/data/pp/humanist-design/postgres"
        target: /var/lib/postgresql/data
        type: bind
    health:
      command: ["pg_isready", "-U", "${env.POSTGRES_USER}", "-d", "${env.POSTGRES_DB}"]
      timeout_seconds: 60

  s3:
    image: "minio/minio:latest"
    pull: if_missing
    phase: infra
    hosts: [p3]
    command: ["server", "/data", "--console-address", ":9001"]
    env:
      source: ".env.s3"
      required:
        - MINIO_ROOT_USER
        - MINIO_ROOT_PASSWORD
    volumes:
      - source: "/data/pp/humanist-design/s3"
        target: /data
        type: bind
    health:
      command: ["curl", "-sf", "http://127.0.0.1:9000/minio/health/ready"]
      timeout_seconds: 60

  convex-backend:
    image: "ghcr.io/get-convex/convex-backend:latest"
    pull: if_missing
    phase: backend
    hosts: [p3]
    env:
      source: ".env.convex.backend"
      required:
        - INSTANCE_NAME
        - INSTANCE_SECRET
        - CONVEX_CLOUD_ORIGIN
        - NEXT_PUBLIC_DEPLOYMENT_URL
        - CONVEX_SITE_ORIGIN
        - SITE_URL
        - POSTGRES_URL
        - AWS_REGION
        - AWS_ACCESS_KEY_ID
        - AWS_SECRET_ACCESS_KEY
        - AWS_S3_FORCE_PATH_STYLE
        - S3_ENDPOINT_URL
        - S3_STORAGE_EXPORTS_BUCKET
        - S3_STORAGE_SNAPSHOT_IMPORTS_BUCKET
        - S3_STORAGE_MODULES_BUCKET
        - S3_STORAGE_FILES_BUCKET
        - S3_STORAGE_SEARCH_BUCKET
    ports:
      - published: auto
        target: 3210
      - published: auto
        target: 3211
    health:
      command: ["curl", "-sf", "http://127.0.0.1:3210/version"]
      timeout_seconds: 90

  convex-dashboard:
    image: "ghcr.io/get-convex/convex-dashboard:latest"
    pull: if_missing
    phase: backend
    hosts: [p3]
    env:
      source: ".env.convex.dashboard"
      required:
        - CONVEX_CLOUD_ORIGIN
        - NEXT_PUBLIC_DEPLOYMENT_URL
        - CONVEX_SITE_ORIGIN
    command_env:
      PORT: "3211"
    ports:
      - published: auto
        target: 3211
    health:
      http: "https://dash.humanist.design/"
      timeout_seconds: 90

  web:
    image: "humanist-design-web:${release}"
    build: web
    phase: frontend
    hosts: [p3]
    ports:
      - published: auto
        target: 80
    health:
      http: "https://humanist.design/"
      timeout_seconds: 60

route_files:
  - source: deploy/caddy/humanist.caddy.tmpl
    host: p3
    dest_name: humanist-design.caddy

hooks:
  after_backend_healthy:
    - name: push convex env
      run: ["bash", "scripts/push-.env.local-to-convex.sh", "prod"]
      env:
        source: ".env.local"
        required:
          - CONVEX_SELF_HOSTED_URL
          - CONVEX_SELF_HOSTED_ADMIN_KEY

    - name: sync convex functions
      run: ["npx", "convex", "dev", "--once", "--env-file", ".env.local"]
      env:
        source: ".env.local"
        required:
          - CONVEX_SELF_HOSTED_URL
          - CONVEX_SELF_HOSTED_ADMIN_KEY

checks:
  smoke:
    - name: frontend
      url: "https://humanist.design/"
      expect_status: 200
    - name: convex version
      url: "https://api.humanist.design/version"
      expect_status: 200
```

## Deployment Order

1. Validate `deploy.yml`, required env files, local tools, and git metadata.
2. Resolve p3 auto ports for frontend, backend `3210`, backend/site `3211`, dashboard, and any exposed S3 admin endpoint if used.
3. Build frontend image locally with only public Vite build args.
4. Save frontend image tar into `.deploy/releases/<release>/images/`.
5. Render compose and Caddy route template.
6. Upload bundle and frontend image tar to p3.
7. Load frontend image on p3.
8. Pull upstream images on p3.
9. Start/update `postgres`, `s3`, `convex-backend`, and `convex-dashboard`.
10. Wait for Postgres, S3, and Convex backend health.
11. Run `scripts/push-.env.local-to-convex.sh prod` or the fixed equivalent.
12. Run `npx convex dev --once --env-file .env.local`.
13. Start/update `web`.
14. Install/reload Caddy route fragment, including `www.humanist.design` redirect to apex.
15. Run smoke checks.
16. Record deployment metadata locally and on p3.

## Implementation Phases

### Phase 1: Capture the working p2 behavior

- Copy the Humanist Caddy route behavior into `../humanist.design/deploy/caddy/humanist.caddy.tmpl`.
- Use p2's dashboard `PORT=3211` pattern.
- Create a p3 Postgres env shape and confirm Convex accepts `POSTGRES_URL`.
- Create a p3 S3-compatible storage env shape and confirm Convex accepts the required `S3_STORAGE_*` and `AWS_*` values.
- Build a manual compose file once on a non-critical path if possible.

### Phase 2: Extend `pp` minimally

- Add `builds` and service-level `build`.
- Add external image pull handling.
- Add multi-image tar save/load.
- Add build args sourced from env.
- Add route file templating.
- Add local hooks with env loading and timeout handling.
- Add phased compose apply or service-group apply.
- Add bind mounts for persistent host paths if not already supported.

### Phase 3: Wire Humanist

- Add production Dockerfile/nginx config to `../humanist.design`.
- Add `deploy.yml`.
- Add env example files.
- Fix or replace the Convex env push script.
- Run `pp plan` from `../humanist.design`.
- Inspect rendered compose and Caddy route output before applying.
- Run first deploy to p3.

### Phase 4: Harden operations

- Pin Convex backend/dashboard image tags after the first known-good deployment.
- Implement Postgres backups under `tickets/humanist-convex/005-add-postgres-backups.md`.
- Add restore notes and rollback expectations.
- Extend `pp status` to read Docker health, route digest, image tags, and p3 persistent volume paths.
- Document Convex dashboard master-key reset/regeneration.

## Remaining Questions

- Which S3-compatible container should be standard for p3: MinIO, Garage, SeaweedFS, or an existing host service?
- Should the S3 admin console be exposed at all, or kept host-local only?
- Which exact bucket names should Humanist use for Convex exports, imports, modules, files, and search?
