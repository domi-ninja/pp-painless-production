# Humanist Convex Deployment User Flow

Run commands from `../humanist.design`.

## Normal Deploy

```sh
pp deploy
```

`pp deploy` handles the full flow:

1. Builds the Vite/nginx frontend image locally.
2. Starts or updates p3 services: Postgres, MinIO, Convex backend, Convex dashboard.
3. Waits for backend health.
4. Installs Caddy routes so `https://api.humanist.design` is reachable.
5. Creates required MinIO buckets.
6. Generates the real Convex admin key from the live backend container with `/convex/generate_admin_key.sh`.
7. Writes `CONVEX_SELF_HOSTED_ADMIN_KEY` into ignored `.env.local`.
8. Pushes `.env.local` into Convex.
9. Runs `npx convex dev --once --env-file .env.local`.
10. Starts or switches the frontend.
11. Reloads Caddy and smoke-checks frontend/API.

## First-Time Env Bootstrap

Run this once before the first deploy, or after changing generated production defaults:

```sh
./scripts/bootstrap-prod-env.sh .env.local
```

This creates generated local production values in ignored `.env.local`. It does not create the Convex admin key; that key is generated from the running backend during deploy.

## Manual Key Refresh

If the Convex admin key needs to be regenerated manually:

```sh
./scripts/refresh-convex-admin-key.sh
```

Then deploy:

```sh
pp deploy
```
