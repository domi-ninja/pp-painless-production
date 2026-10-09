# Deployment examples

Copy a template into your app, replace the project, host and domains, and merge the ignore rules. Application source and dependency lockfiles are not included.

- [Simple website](simple-website/README.md) builds a pnpm frontend and serves it with nginx.
- [Stateful Go website](stateful-go-website/README.md) runs a Go app with SQLite, uploads and a persistent volume.
- [Convex website](convex-website/README.md) runs Convex with Postgres, SeaweedFS and a private dashboard.

Run `pp plan` before deploying. Use separate data paths, ports and credentials for independent environments. Preserve an existing app's `.deploy` state and data names when updating it. Never copy existing state or secrets into a new deployment.

Read [image updates](images.md) and [backups and isolated restore](backups.md) before changing stateful services.
