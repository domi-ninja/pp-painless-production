# Back up and test a restore

pp retains container releases, not database backups. Store backups separately from the host and encrypt them. A backup contains database contents, uploads and often credentials. Use `umask 077` before creating local files.

Before changing storage or database versions, test a restore with the same image version in an isolated environment. Record the image digest and the data paths with the backup. Keep the private env file and encryption keys in your secret store, separate from the database archive.

## SQLite example

Find the exact container for the selected project and environment through its pp labels. Require one match; never use a substring of a container name.

```sh
docker ps --filter label=pp.project=stateful-go-website \
  --filter label=pp.environment=prod --filter label=pp.service=web
```

The example includes `sqlite3`. An online database-only backup uses SQLite's backup API:

```sh
docker exec CONTAINER sqlite3 /data/data.db '.backup /data/backup.db'
umask 077
docker cp CONTAINER:/data/backup.db ./database.backup
```

This does not synchronize uploads with the database. For a complete copy, stop the app with `pp down`, inspect its actual named volume, and archive that volume read-only on the host. `pp down` preserves volumes and port reservations. Capture `/data/app.toml`, uploads and the database together while writes are stopped. Redeploy after the copy.

Restore into a new disposable volume, never over the live one. Set ownership to UID/GID 10001. Run the pinned app image with that volume, test-only credentials, loopback-only ports and no public route. Check `PRAGMA integrity_check`, `/health`, a database read and an uploaded file. Test both the new migrations and the older app's compatibility before relying on container rollback. Delete only the disposable test resources after verification.

## Convex example

Postgres and object storage must describe the same point in time. Take the app offline and stop writes before a coordinated backup. The simplest cold backup is `pp down`, then an archive of the configured Postgres and SeaweedFS bind directories on the host. Do not guess their paths from the project name; existing installations can use earlier paths. Confirm the expected data device is mounted first.

Preserve ownership and file modes. Copy both directories and the private env file to protected storage, then redeploy. A live copy of a Postgres data directory is not a valid cold backup. For logical backups, use `pg_dump` while the backend is stopped and coordinate the object-store snapshot. Keep credentials in the container environment or stdin, never in command arguments.

Restore both stores into new directories on a separate test host using the same pinned images. Use a separate pp project and environment, separate host ports and test credentials. Keep public routes disabled. Test a Convex query, a mutation and retrieval of an uploaded object. A working empty backend is not evidence that a restore succeeded. Verify representative existing data before moving production traffic.

Changing Postgres major versions, storage implementations or credential formats needs a separate migration plan. Do not initialize fresh data or generate new credentials over an existing installation to make a failed restore start.
