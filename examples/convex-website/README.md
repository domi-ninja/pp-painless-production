# Convex website

This template deploys a Vite website, Convex backend and private dashboard, Postgres and MinIO on one host. Copy it into an existing Convex app with a `build:web` script, a pinned `convex` dependency and a pnpm lockfile. Merge the ignore rules. Replace the project name, SSH target and site/API hostnames in [deploy.yml](deploy.yml) and the [Caddy template](deploy/caddy/convex-website.caddy.tmpl).

```sh
pnpm install --frozen-lockfile
bash scripts/bootstrap-prod-env.sh convex.example.com --project convex-website
pp plan
pp deploy
pp status
```

Bootstrap writes `.env.prod` with mode 0600. It preserves existing credentials, names, URLs and application keys. The [example env file](.env.prod.example) lists the keys; its placeholders are not usable credentials. The hooks use the app's installed Convex CLI and pass `.env.prod` explicitly. They never download a CLI during deployment.

The infrastructure-ready hook creates the buckets and a MinIO account limited to those buckets before starting the backend. Only MinIO receives root credentials. After backend health succeeds, hooks refresh the admin key, set `SITE_URL`, `JWT_PRIVATE_KEY` and `JWKS`, then deploy functions. This allowlist assumes Convex Auth. Adapt it to the app's provider; never upload infrastructure credentials or bulk-copy the env file.

## Dashboard

The backend and dashboard bind to host loopback ports 13210 and 13211. From your workstation:

```sh
ssh -N -L 13210:127.0.0.1:13210 -L 13211:127.0.0.1:13211 deploy@example.com
```

Open `http://127.0.0.1:13211`. The browser connects to the backend through port 13210. Neither dashboard nor admin API has a public Caddy route. For another environment on the same host, choose another port pair and change `NEXT_PUBLIC_DEPLOYMENT_URL` and the tunnel to match.

## Credential rotation and recovery

Rotation is explicit:

```sh
bash scripts/bootstrap-prod-env.sh convex.example.com --project convex-website --rotate
pp deploy
```

It changes the instance, database, MinIO and JWT secrets, removes the cached admin key, and saves `.env.prod.previous` before replacing the file. It preserves database, bucket and storage account names and other app-owned keys. The Postgres wrapper synchronizes the password on existing volumes. Deploy that wrapper with the existing credentials before rotating an older installation.

Keep the recovery copy until the app and database are verified. A second rotation refuses to overwrite it. To restore credentials:

```sh
bash scripts/bootstrap-prod-env.sh --restore
pp deploy
```

Restore preserves the backup and does not contact the host. Deploy applies the restored inputs and function variables. Rotation can interrupt requests and invalidate auth sessions. pp cannot undo function or credential changes through container rollback; it refuses automatic recovery for releases with hooks. Recover the app explicitly and verify it before archiving the private backup.

## Existing installations

Preserve the configured data paths when adopting this template. New installations use `/data/pp/${project}/${environment}/postgres` and `/data/pp/${project}/${environment}/s3`. Changing an existing bind path starts with different data. Keep the old paths until you have copied and verified the data deliberately.

Helpers require `PP_PROJECT`, `PP_ENVIRONMENT`, `PP_HOST` and `PP_ENV_FILE`. pp sets them for hooks. Manual runs must supply the same identity; container selection rejects missing or ambiguous matches. See the [backup guide](../backups.md) before changing storage or infrastructure versions.

The [upstream self-hosting guide](https://github.com/get-convex/convex-backend/tree/main/self-hosted) describes backend and dashboard configuration. The CLI's [deployment implementation](https://github.com/get-convex/convex-js/blob/main/src/cli/deploy.ts) documents explicit env-file selection.
