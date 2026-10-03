# Fleet maintenance, 2026-10-03

Scope: pp deployments on p3, their local source checkouts on seth, the installed pp CLI, and pp examples. This is not a blanket application dependency upgrade. Production environments were consumed by deployment scripts, never printed or committed. No Drive credentials were rotated during this pass.

## Shared tooling

Example changes landed in pp commit `6201162`, pushed to GitHub and Forgejo. The installed CLI at `/root/go/bin/pp` is built from the maintained main checkout. All Go tests pass, including a new test that validates every example against the current configuration loader.

The Convex example bootstrap was also exercised in an isolated temporary fixture: initial generation, mode 0600, rotation backup, byte-for-byte restoration, and `pp plan` all passed. Generated test credentials were removed; no example deployment contacted a server.

The Convex example now uses one `.env.local`, an explicit function-variable allowlist, local credential generation with a recovery copy, Postgres password synchronization, a private dashboard and exact-origin CORS. Simple and stateful examples use release-specific image tags and public smoke checks. No pp runtime redesign was made.

The CLI includes working-tree changes in builds. Commit reviewed changes first; otherwise use an isolated worktree of the intended commit. Do not copy another deployment's `.deploy/` directory or rename populated volumes. Existing legacy resource IDs remain in use after state migration.

## Deployment results

| Project | Source | Result |
| --- | --- | --- |
| Drive 2 | `drive2.humanist.design`, main `ee6c577` | Deployed `20261003T133026Z-ee6c577`. Convex env allowlist and functions pushed to api.drive2.humanist.design; both public checks passed. |
| Blog | `blog.domi.ninja`, main `b7cb724` | Deployed `20261003T133148Z-b7cb724`; public check passed. GitHub updated. Forgejo remote unavailable. |
| Kunst | `kunst.bio`, main `77911f0` | Deployed `20261003T134024Z-77911f0`; public check passed. Environment files excluded from the static image. No Git remote configured. |
| Flags | `flags-game`, master `534daf4` | Deployed `20261003T133245Z-534daf4`; standalone and /flags/ checks passed. Both remotes updated. |
| Uptime Kuma | `kuma.websites.fail`, main `c15b14c` | Restored service, then successfully ran the real systemd updater. Deployed `20261003T133416Z-c15b14c` with current Kuma image digest `c74379ac4509…`; public check passed and remote main updated. |
| Audio | `audio.domi.ninja`, main `a39c238` | Deployed `20261003T133746Z-a39c238`; public health check passed. No Git remote configured. |
| Gotths | `gotths-example`, main `234dd71` | Deployed `20261003T133830Z-234dd71`; public database health check passed. Existing deployment files committed, docs corrected, upstream docs merged; Go tests pass. |
| yt-music | `yt-music`, main `f74c434` | Deployed `20261003T134501Z-f74c434` from clean main. Authenticated frontend and health checks passed. App.tsx edit restored with matching diff checksum; temporary stash removed. Remote main updated. |
| Rogue | `roguelike-roguelike` deployment worktree, `9bd5f58` | Deployed `20261003T134909Z-9bd5f58`; authentication-required check passed. Deployment branch pushed to self-hosted remote. Main is a different development line without deploy.yml. |
| Canaries | `canaries`, main `38bd17c` | Release-specific image tags committed and pushed. All three runner tests passed. Final run finished at 13:54:21 UTC: pp build, Blog and Gotths all PASS, including Gotths database and CRUD checks. |
| Original Drive | `drive.humanist.design`, main `17950ac` | Left running unchanged pending branch choice. See below. |
| Humanist | `humanist.design`, main `c0780e8` | Left running unchanged pending permission to override its no-commit rule for script fixes. |
| Discord Archive | `discord-archive`, main `4b059d3` | Fixed copied yt-music image name and pushed. Not deployed: current README says deployment is not set up; unrelated uncommitted app work remains. |

Final application inspection found no stopped or unhealthy pp containers on p3. MinIO services without a configured Docker healthcheck were running; Drive 2's bucket hook also passed during this sweep. Drive 2, original Drive and Humanist each returned HTTP 200 for their frontend and API version endpoints. Original Drive and Humanist being reachable does not mean their deferred maintenance is complete.

## Confirmed issues and decisions still needed

- Original Drive and Drive 2 share the `convex-drive` remote. Domi explicitly selected that remote for Drive 2 main. Do not pull its new main into the original Drive deployment: the deployment identifiers and data paths target Drive 2. A separate original-Drive deployment branch is proposed, awaiting approval. The pre-existing deletion of vercel.json is untouched.
- Humanist's AGENTS.md prohibits agent commits. Its current environment uploader bulk-uploads the source environment rather than a function-only allowlist, and its bucket helper passes credentials in process arguments. Updating these requires preserving Humanist's actual Better Auth and email-provider configuration, not copying Drive 2's three-variable allowlist. Permission to commit this maintenance is pending.
- The Blog Forgejo remote returned repository-not-found. GitHub is synchronized and local main now tracks github/main; no replacement remote was invented.
- Rogue's GitLab fetch failed with public-key authentication denied. Its self-hosted fetch succeeded, and the maintained deployment branch was pushed there. That branch is intentionally not merged into the separate main development line.
- `www.kunst.bio` has no DNS record. `kunst.bio` resolves and passed HTTPS verification. DNS was not changed.
- Audio and Kunst have no configured Git remotes, so their commits are local only.
- Discord Archive's deployment image name was a confirmed copy-paste collision. Its unfinished application changes were not committed or released as maintenance.

## Kuma incident

The September weekly updater failed with `$HOME is not defined` during both deployment and rollback. After copying the backup back into place, it left MariaDB and Kuma stopped. The service now declares `User=root`, which supplies the login environment. The updater checks that a valid home exists and runs `pp plan` before stopping services.

A transient systemd run with the same user and working directory passed the updater's `--check`. The first restoration smoke check exposed a startup race: pp starts frontend services without waiting for health, and Kuma briefly returned 502. A bounded retry now covers startup. The subsequent pinned-release deployment passed and both services recovered. No database volumes were deleted. A stopped snapshot protects the available-image update.

The actual systemd update subsequently completed successfully with the available image. Snapshot `20261003T133357Z` remains under `/data/pp/uptime-kuma/backups/` for recovery. Audio showed the same startup race; its smoke check now retries transient startup failures too.

## Retention and preserved work

pp's configured retention removed older release bundles and some owned image tags during deployment. Those bundles are no longer rollback targets; they can be rebuilt from source but not recovered as the original bundle. Persistent application data and database volumes were preserved. Legacy image tags without ownership manifests were left alone.

Rogue's first attempt stopped before building because the root filesystem had less than the required 1 GiB free. Normal pp cleanup had no further expired releases to remove. A targeted prune of the inspected pp-owned builder `pp-managed-94a6b4475803` reclaimed 6.029 GB of reproducible build cache, leaving 6.7 GiB free at that point. No generic Docker system prune or volume prune was used. The data volume had ample space; moving Docker storage is outside this maintenance pass.

An isolated yt-music release worktree was tried but not deployed. pp correctly rejected environment/state symlinks outside that checkout. The temporary worktree and its private environment copy were removed. The release instead used the original state directory and a targeted temporary stash of App.tsx; its diff checksum matched after restoration. Do not bypass pp's state-path protections to share state between worktrees.

The machine has other Git checkouts, many with uncommitted work and no pp configuration. No blanket merge, branch rename, dependency upgrade or secret regeneration was attempted in those unrelated repositories. Whether to pull every unrelated repository is a separate scope question awaiting an answer.
