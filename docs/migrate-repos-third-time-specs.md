# Repository migration

The helper migrates local branches under `/root/repos` to private repositories owned by `domi-ninja` on `git.domi.ninja`.

Create a plan without changing local remotes or contacting Forgejo:

```sh
node scripts/migrate-repos-third-time/migrate-repos.cjs --plan /tmp/migration-plan.json
```

Inspect every source path, existing upstream, destination owner/repository, and branch SHA in the plan. A stale upstream can suggest the wrong destination. Correct the local upstream and generate a new plan if a mapping is wrong. Remove entries you do not want to migrate.

Provide an operator-managed token through `FORGEJO_TOKEN`, then apply the reviewed plan:

```sh
node scripts/migrate-repos-third-time/migrate-repos.cjs --apply /tmp/migration-plan.json
```

The token needs repository write access and the user/organization scopes required to create repositories for the destination owner. Create it for this migration and revoke it in Forgejo after use, including failed runs. The script never creates an access token or logs its value.

Before changing remotes or pushing, the helper verifies that the local mapping and branch SHAs still match the plan and that Forgejo returns the expected private owner/repository. It rejects duplicate destinations. Existing non-Forgejo upstreams are renamed using their domain, and local branches are pushed to the verified SSH upstream. It stops on the first failure; inspect any partial migration before generating another plan.
