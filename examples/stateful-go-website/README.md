# Stateful Go website

Copy this template into a Go app with `cmd/server`, frontend assets and Goose migrations. Replace the host and domain in [deploy.yml](deploy.yml), merge the ignore rules, and create `.env` with a real `JWT_SECRET`. Then run `pp plan` and `pp deploy`.

The [Dockerfile](Dockerfile) builds for the selected target architecture. The runtime runs as UID/GID 10001 and stores its SQLite database, uploads and configuration in the named volume at `/data`. A fresh Docker volume inherits that directory's ownership. Before adopting this image with an old root-owned volume, stop the app, back it up and change only that volume's ownership to 10001:10001.

The [entrypoint](docker-entrypoint.sh) copies `app.prod.toml` only when `/data/app.toml` does not exist. Later image changes do not replace the stored config. Edit the stored file deliberately or add an application config migration. Packaged frontend assets and migration files refresh on each start.

Goose runs SQLite migrations before every container start. These migrations run outside pp's migration record. Container rollback cannot undo them. Use compatible schema changes and a verified database backup before deploying. Never assume that restarting an older image restores old data.

See [image updates](../images.md) and [backup and isolated restore](../backups.md).
