# Simple website

Use this example for a static site or frontend that builds into one HTTP container. `pp` builds the image, copies it to one server, assigns a localhost port, checks the container, and publishes it through Caddy.

The example contains:

- [`deploy.yml`](deploy.yml) for the image build, server placement, health check, and public route
- [`Dockerfile`](Dockerfile) for a Node and pnpm build served by nginx
- [`.dockerignore`](.dockerignore) and [`.gitignore`](.gitignore) for build output, local deployment state, and secrets

Copy these files into the application repository, then replace `deploy@example.com` and `simple.example.com`. The Dockerfile expects `package.json`, `pnpm-lock.yaml`, and a build that writes the site to `dist/`.

Run `pp plan` to validate the config, then `pp deploy` to release it.

Pin pnpm in the app's `packageManager` field. Declare only dependency build scripts the app needs in `pnpm-workspace.yaml`. The image does not approve every dependency's install scripts. See [image updates](../images.md).
