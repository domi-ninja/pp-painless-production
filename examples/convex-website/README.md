# Convex website

Use this example when you want to self-host a Convex application on one Docker server. Convex is handy for side projects because it combines a database, reactive queries, mutations, and backend functions. This example keeps those conveniences while running the services on your own server.

The deployment includes the website, Convex backend and private dashboard, Postgres, and MinIO. Hooks create buckets, refresh the admin key, push only the application-owned environment allowlist, and sync Convex functions. This template assumes standard Convex Auth with JWT keys; adapt its allowlist for your application's actual auth provider.

The example contains:

- [`deploy.yml`](deploy.yml) for the services, build, volumes, hooks, and smoke checks
- [`Dockerfile`](Dockerfile) and [`nginx.conf`](nginx.conf) for the website
- [`deploy/caddy/convex-website.caddy.tmpl`](deploy/caddy/convex-website.caddy.tmpl) for site/API routes and exact-site CORS; the dashboard has no public route
- [`scripts/`](scripts/) for production environment setup and post-start deployment work
- [`.env.local.example`](.env.local.example) as the placeholder-only generated environment schema
- [`deploy/postgres/`](deploy/postgres/) to synchronize passwords on existing database volumes before accepting network connections

Copy these files into a Vite/Convex application with a `build:web` package script, `pnpm-lock.yaml`, and `pnpm-workspace.yaml`. Merge the ignore rules. Replace `convex-website` in project/image/data/route identifiers, `deploy@example.com`, and every `convex.example.com` hostname. Set the application's own Git remote. Do not copy deployment state or existing environment files.

```sh
pnpm add jose
bash scripts/bootstrap-prod-env.sh convex.example.com
pp plan
pp deploy
pp status
```

The canonical ignored file is `.env.local`, mode 0600. The hostname argument derives site/API URLs; the literal Caddy hostnames and smoke URLs must match it. Instance and database names derive from the project slug. The dashboard remains loopback-only. Add application HTTP routes to Caddy's HTTP-action matcher as needed.

`scripts/push-env-to-convex.sh` uploads only `SITE_URL`, `JWT_PRIVATE_KEY`, and `JWKS`. Infrastructure and admin secrets never belong in that allowlist. Add provider-specific values only after checking their consumers. The CLI may add derived `VITE_CONVEX_SITE_URL` output. Do not enable bulk upload or remote pruning.

Routine deploys reuse credentials. Bootstrap deliberately regenerates all local credentials and preserves data-bearing names. Before rotating an older deployment, first deploy the Postgres startup behavior with its existing environment. Bootstrap saves `.env.local.previous` and refuses to overwrite an outstanding recovery copy. Failed rotation recovery is `bash scripts/bootstrap-prod-env.sh --restore`, followed by `pp deploy`, which also restores function variables. This is explicit recovery, not a zero-downtime transaction. Bare `pp rollback` uses the current local hook environment, so it is insufficient across credential changes.

Keep the recovery copy until verification, then archive it securely or remove it. Never rename populated volumes, databases, or buckets as part of a routine update. Do not generate fresh secrets merely to try this example against an existing deployment.
