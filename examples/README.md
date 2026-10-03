# Deployment examples

These examples started from sibling repositories, then replaced project-specific names and domains with consistent placeholders. Replace `deploy@example.com` and the `example.com` domains before using them.

- `simple-website/` shows one built web image at `simple.example.com`.
- `convex-website/` shows the more complex setup needed to run Convex in self-hosted mode. It lets a side project use Convex's database and backend functions while hosting the services on your own server.
- `stateful-go-website/` shows a required secret, persistent volume, migrations, and a custom entrypoint at `stateful.example.com`.

Application source and dependency lockfiles are not duplicated here. No production environment file or secret is included.

Build tags use `${release}` so a new release does not reuse a mutable commit tag. Public smoke checks complement container health checks. Start with `pp plan`, deploy from a reviewed checkout, and compare `pp status` with the intended commit afterward. Preserve the app's existing `.deploy` state and data identifiers when updating it; discard copied state only when creating an independent deployment.

The Convex example uses a narrow function-environment allowlist, private dashboard, and recoverable credential rotation. Read its README before adapting an existing installation; do not copy fresh keys over a running application's environment.
