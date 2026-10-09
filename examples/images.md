# Image and dependency updates

Examples pin upstream images by multi-platform digest. Built app images use `${release}`. A pin makes an update reviewable; it does not guarantee that its contents are secure. Package installs inside a build still resolve against upstream repositories.

| Component | Pin location | Update source |
| --- | --- | --- |
| Node 22 | Website Dockerfiles | [Node image](https://hub.docker.com/_/node) |
| nginx 1.30.5 | Website Dockerfiles | [nginx image](https://hub.docker.com/_/nginx) |
| Go 1.26.9 | Go and storage Dockerfiles | [Go releases](https://go.dev/dl/) |
| Debian bookworm | Runtime Dockerfiles | [Debian image](https://hub.docker.com/_/debian) |
| Postgres 16.15 | Convex Postgres Dockerfile | [Postgres image](https://hub.docker.com/_/postgres) |
| Convex backend and dashboard | Convex deploy.yml | [Convex self-hosting](https://github.com/get-convex/convex-backend/tree/main/self-hosted) |
| SeaweedFS 4.48 | Convex storage Dockerfile | [SeaweedFS releases](https://github.com/seaweedfs/seaweedfs/releases) |
| Goose 3.26.0 and Tailwind 3.4.19 | Go Dockerfile | [Goose releases](https://github.com/pressly/goose/releases), [Tailwind releases](https://github.com/tailwindlabs/tailwindcss/releases) |

The Convex template uses SeaweedFS with credentials restricted to its declared buckets. Only its S3 endpoint is reachable over the application network; internal storage APIs bind to loopback. It runs as UID/GID 10001. The single-process store has no replication and still needs backups.

MinIO community is [unmaintained](https://github.com/minio/minio#readme). Existing MinIO directories cannot be opened by SeaweedFS. Keep the old deployment and data intact until an isolated transfer and restore have passed. Follow the [Convex storage migration guide](https://github.com/get-convex/convex-backend/blob/main/self-hosted/advanced/s3_storage.md) for export/import or transfer every existing bucket through S3 while writes are stopped. Verify database records and uploaded files before changing production endpoints.

Before an update, read the upstream changelog and storage compatibility notes. Verify an index containing the host's architecture:

```sh
docker buildx imagetools inspect IMAGE:VERSION
```

Copy the index digest into the Dockerfile or config. Update Convex backend and dashboard together. Review source commit changes for source-built tools. Build the app for its host platform, scan the resulting images, and test migrations and restore in an isolated environment. Back up state before infrastructure upgrades. Major Postgres upgrades require their own data migration; changing an image tag is insufficient.

Pin pnpm through the app's `packageManager` field and keep its lockfile. Configure approved dependency scripts in `pnpm-workspace.yaml`; do not enable blanket build approval. The examples require the app's own code and cannot run as standalone apps.
