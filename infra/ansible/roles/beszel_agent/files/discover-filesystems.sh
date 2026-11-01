#!/usr/bin/env bash
set -euo pipefail
# One mount per real filesystem. Exclude virtual/container mounts and the root,
# which Beszel always monitors. Re-evaluate whenever the service starts.
mounts=$(findmnt -rn -t ext2,ext3,ext4,xfs,btrfs,zfs -o MAJ:MIN,TARGET | awk '!seen[$1]++ && $2 != "/" {print $2}' | paste -sd, -)
printf 'EXTRA_FILESYSTEMS=%s\n' "$mounts" > /run/beszel-agent/filesystems.env
