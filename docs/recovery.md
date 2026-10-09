# Recovery

A container release is not a data backup. Back up databases, uploads and application configuration separately, and test a restore before changing them. pp cannot make arbitrary migrations or application hooks reversible.

## Saved inputs

New releases save a private `recovery.json` and selected `migration.env` when needed. Recovery uses recorded identity, hosts, phases, HTTP checks, migration arguments and the exact migration image ID. It needs the original Compose files, runtime env files, recorded route files, image locks and built-image archives. Preserve the complete release directory when moving a checkout.

Rollback reads identity without requiring today's build files, env files or Git history. It never runs today's hooks or command checks against older containers. Releases containing those scripts require explicit application recovery before restoring containers. A legacy release without a saved migration contract requires manual database recovery. Simple legacy container-only releases remain recoverable.

## Failed deployment

A failed forward migration may have changed data partially. pp records failure and stops; it does not guess an undo command. Check the database and the migration's transaction policy before retrying.

After a completed migration followed by failed apply, pp validates the recovery target, then runs the saved database rollback command before starting older containers. A failed database revert stops container recovery. A `restore_plan` always requires manual action. Existing services can still be running during migration and revert; commands must handle that compatibility.

Without a previous release, there are no older containers to restore. Inspect any partial first deployment before retrying or using down. pp removes its owned migration container after execution, including timeout or cancellation, and reports cleanup failure with the container name. It never removes a container whose ownership label differs. Cancellation stops pp subprocesses, but does not prove that remote services or a database operation made no changes.

## Explicit rollback

```sh
pp status
pp rollback --config deploy.prod.yml
pp status --config deploy.prod.yml
```

Check the intended project/environment and observed hosts first. After a database rollback, another rollback targeting the newer release is refused. Redeploy the intended version with its forward migration policy; pp will not run an older database undo command to move forward. Rollback reapplies saved bundles and performs their HTTP checks. It records recovery checks separately from the original deployment checks. Missing required artifacts or unavailable saved migration images fail before recovery changes data.

Restoring containers can fail when upstream images were manually removed, saved ports were released, or a required host is offline. Keep current and previous releases and their image references available. Use status to compare saved success with observed containers.

## Application recovery

Convex schema/function pushes and credential rotation need an app-specific recovery procedure. Restore the private saved env inputs and deploy the intended application version. The [Convex template](../examples/convex-website/README.md#credential-rotation-and-recovery) preserves a credential recovery copy until verification.

The [Go example](../examples/stateful-go-website/README.md) runs SQLite migrations in its entrypoint, outside pp's migration record. An older image does not restore an older database. Prefer schema changes compatible with both versions, or use a tested isolated restore before switching traffic.
