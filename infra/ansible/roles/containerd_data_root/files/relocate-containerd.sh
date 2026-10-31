#!/usr/bin/env bash
# Move containerd's offline store, retain the source until containers recover.
set -Eeuo pipefail
umask 077
target=${1:-/data/containerd}
[[ $EUID == 0 ]] || { echo 'Run as root' >&2; exit 1; }
[[ $target == /data/containerd ]] || { echo 'Expected /data/containerd' >&2; exit 1; }
mountpoint -q /data || { echo '/data must be mounted' >&2; exit 1; }
exec 9>/run/pp-containerd-migration.lock
flock -n 9 || { echo 'Migration already running' >&2; exit 1; }
deployment_user=${2:-deploy}
deployment_home=$(getent passwd "$deployment_user" | cut -d: -f6)
if [[ -n $deployment_home && -f $deployment_home/.pp/operation.lock ]]; then
 [[ ! -L $deployment_home/.pp/operation.lock ]] || exit 1
 exec 8>"$deployment_home/.pp/operation.lock"
 flock -n 8 || { echo 'A pp deployment is running; retry after it finishes' >&2; exit 1; }
fi
source=$(containerd config dump | sed -n "s/^root = ['\"]\(.*\)['\"]$/\1/p")
if [[ $source == "$target" ]]; then echo 'unchanged'; exit 0; fi
[[ $source == /var/lib/containerd && -d $source && ! -L $source ]] || { echo 'Unexpected source; inspect manually' >&2; exit 1; }
[[ ! -e $target ]] || { echo 'Destination exists; inspect before retrying' >&2; exit 1; }
backup=/data/containerd-migration-$(date -u +%Y%m%dT%H%M%SZ)
mkdir -p "$backup" "$target"
cp -a /etc/containerd/config.toml "$backup/config.toml"
docker ps -q >"$backup/running-containers"
docker ps --format '{{.Names}} {{.Status}}' >"$backup/before.txt"
mkdir -p /etc/systemd/system/containerd.service.d
if [[ -e /etc/systemd/system/containerd.service.d/pp-data.conf ]]; then cp -a /etc/systemd/system/containerd.service.d/pp-data.conf "$backup/pp-data.conf"; fi
printf 'Pre-copying containerd store to %s\n' "$target"
rsync -aHAXx --numeric-ids --exclude /tmpmounts/ --exclude /io.containerd.runtime.v2.task/ "$source/" "$target/"
rollback() {
 trap - ERR
 echo 'Migration failed; restoring original configuration and store' >&2
 systemctl stop docker.socket docker.service containerd.service || true
 cp -a "$backup/config.toml" /etc/containerd/config.toml
 if [[ -e $backup/pp-data.conf ]]; then cp -a "$backup/pp-data.conf" /etc/systemd/system/containerd.service.d/pp-data.conf; else rm -f /etc/systemd/system/containerd.service.d/pp-data.conf; fi
 systemctl daemon-reload
 systemctl start containerd.service docker.service docker.socket
 xargs -r docker start <"$backup/running-containers"
 echo "Original store retained at $source; inspect $target before retrying" >&2
 exit 1
}
trap rollback ERR
printf 'Stopping running containers for final copy\n'
xargs -r docker stop --time 60 <"$backup/running-containers"
systemctl stop docker.socket docker.service containerd.service
rsync -aHAXx --numeric-ids --delete "$source/" "$target/"
# rsync verifies transferred data with its transfer checksum. Avoid a second full
# store reread while applications are down. The source stays until health passes.
{ printf 'root = "%s"\n' "$target"; sed '/^root[[:space:]]*=/d' "$backup/config.toml"; } >/etc/containerd/config.toml
printf '[Unit]\nRequiresMountsFor=/data/containerd\n' >/etc/systemd/system/containerd.service.d/pp-data.conf
containerd config dump >/dev/null
systemctl daemon-reload
systemctl start containerd.service docker.service docker.socket
xargs -r docker start <"$backup/running-containers"
deadline=$((SECONDS + 180))
while :; do
 ready=true
 while read -r id; do
  status=$(docker inspect --format '{{.State.Running}} {{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$id")
  [[ $status == 'true healthy' || $status == 'true none' ]] || ready=false
 done <"$backup/running-containers"
 $ready && break
 (( SECONDS < deadline )) || { echo 'Container health did not recover' >&2; false; }
 echo 'Waiting for container health checks'
 sleep 5
done
[[ $(containerd config dump | sed -n "s/^root = ['\"]\(.*\)['\"]$/\1/p") == "$target" ]]
# The source contains only the now-inactive verified copy. Refuse mounted leftovers.
if findmnt -rn -o TARGET | awk -v p="$source" '$0==p || index($0,p"/")==1 {found=1} END {exit !found}'; then
 echo 'Old source still has mounts; keeping it for manual review' >&2
 exit 1
fi
trap - ERR
printf 'Containers recovered; removing verified old store\n'
rm -rf --one-file-system "$source"
mkdir -p "$source"
docker ps --format '{{.Names}} {{.Status}}' >"$backup/after.txt"
printf 'Migration complete. Audit: %s\n' "$backup"
df -h / /data
