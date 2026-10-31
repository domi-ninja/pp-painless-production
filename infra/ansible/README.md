# Prepare a pp application host

This playbook prepares Ubuntu 22.04+ servers for application deployment with `pp`. Run it when adding a host or changing its system configuration, not for every application release.

It configures SSH access, firewall rules, Docker and optional Caddy or persistent storage. [Git hosting and CI runners](https://git.domi.ninja/domi-ninja/git-host-ansible) have their own repository and playbooks. Enabling their old role flags here fails before provisioning.

## First run

Install Ansible on your workstation, then copy the examples:

```sh
cd infra/ansible
cp inventory/prod.example.yml inventory/prod.local.yml
cp host_vars/prod.example.yml host_vars/prod-1.local.yml
```

Edit the inventory's IP and bootstrap SSH user. Set your public key and email in the ignored host-vars copy. Replace `prod-1` everywhere if your inventory uses another hostname.

```sh
ansible-playbook -i inventory/prod.local.yml --limit prod-1 \
  -e @group_vars/prod_servers.yml \
  -e @host_vars/prod-1.local.yml \
  playbooks/prod-server.yml
```

Local overrides are loaded explicitly, not discovered from their `.local.yml` filename. Keep credentials in ignored files or Ansible Vault. Read [Getting started](../../docs/GETTING_STARTED.md#2-prepare-a-host-usually-once) for SSH lockout precautions and the additional proxy permissions `pp` needs.

After verifying a fresh `deploy` login, update `ansible_user` in the inventory for subsequent runs.

## Settings to review

- `admin_ssh_public_keys`: replace the placeholder with your public key.
- `ssh_port` and `ufw_allowed_tcp_ports`: keep SSH and firewall settings aligned.
- `docker_users`: grant Docker access to the deployment account.
- `reverse_proxy_enabled`: enable host-level Caddy for public HTTPS routes.
- `data_volume_enabled` and `docker_data_root_enabled`: disabled by default. Verify devices and back up existing data before changing storage.

Docker data-root moves require direct, unaliased filesystem mounts. The role rejects nested roots, bind mounts and subvolume aliases before stopping Docker or copying data.

The retained Coolify role is for legacy hosts. Leave it disabled for new pp setups.

## Existing Git-host configuration

Forgejo, Forgejo runner, Woodpecker and runner-isolation roles moved to the sibling `git-host-ansible` repository. Shared baseline roles were copied so both repos work independently. Existing ignored inventory and credential files were left untouched; they were not pushed to either repository. Move the relevant host settings into the new repo before using its playbooks, and never provision an existing server with placeholder secrets.

## Containerd storage and cache cleanup

Docker's containerd image store is separate from Docker's data root. Enabling
`docker_data_root_enabled` also enables `containerd_data_root_enabled` by default.
The default destinations are `/data/docker` and `/data/containerd`. The containerd
helper currently supports the standard `/var/lib/containerd` to `/data/containerd`
move and requires `/data` to be mounted. Override `containerd_data_root_enabled`
to false only when containerd storage is managed separately.

The Bash helper pre-copies the store, stops the running containers and both
daemons, synchronizes the offline store with rsync, which verifies transferred data
with transfer checksums. It saves the original configuration and running-container IDs under
`/data/containerd-migration-TIMESTAMP`. It installs a containerd mount dependency,
restarts the services and previously running containers, and waits for their health checks
before removing the old copy. If the switch fails, it
restores the old configuration and restarts against the retained source. Inspect
a partial destination before retrying. Once the old copy is removed, restoring
the old path requires another offline copy; the configuration backup alone is
not a data backup.

Provisioning also enables `pp-build-cache-cleanup.timer`. It runs daily with up
to one hour of jitter and prunes unused default-builder cache older than seven
days, retaining up to 10 GB under Docker's keep-storage policy. It does not prune
images, containers, volumes, or pp rollback releases. Separate buildx builders
retain their own BuildKit GC policies. Set `docker_cache_cleanup_enabled: false`
to disable the timer. Use `journalctl -u pp-build-cache-cleanup` to inspect runs.
