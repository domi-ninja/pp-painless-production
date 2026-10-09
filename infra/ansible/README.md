# Prepare a pp application host

This playbook prepares Ubuntu 22.04+ servers for application deployment with `pp`. Run it when adding a host or changing its system configuration, not for every application release.

It configures SSH access, firewall rules, Docker and optional Caddy or persistent storage. Provision Git hosting and CI runners separately. Enabling their old role flags here fails before provisioning.

## First run

Install Ansible on your workstation, then copy the examples:

```sh
cd infra/ansible
python3 -m pip install -r requirements.txt
ansible-galaxy collection install -r requirements.yml
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

Local overrides are loaded explicitly, not discovered from their `.local.yml` filename. Keep credentials in ignored files or Ansible Vault. Read [Getting started](../../docs/GETTING_STARTED.md#2-prepare-a-host-usually-once) for SSH lockout precautions and the paths `pp` needs.

After verifying a fresh `deploy` login, update `ansible_user` in the inventory for subsequent runs.

## Settings to review

- `admin_ssh_public_keys`: replace the placeholder with your public key.
- `ssh_port` and `ufw_allowed_tcp_ports`: keep SSH and firewall settings aligned.
- `docker_users`: grant Docker access to the deployment account.
- `reverse_proxy_enabled`: enable host-level Caddy for public HTTPS routes.
- `data_volume_enabled` and `docker_data_root_enabled`: disabled by default. Verify devices and back up existing data before changing storage.

Coolify provisioning is unsupported. Its old enable and proxy-stop flags fail before provisioning. Migrate existing edge services explicitly before giving Caddy ports 80/443.

## SSH keys and firewall changes

The playbook owns the admin account's complete `authorized_keys` file. Removing a key from `admin_ssh_public_keys` revokes it on the next run. It rejects placeholders, invalid keys and RSA keys smaller than 2048 bits before changing the host. Keep a bootstrap connection open while testing new access.

The managed SSH fragment loads before cloud defaults. The playbook checks `sshd -T` for the admin account and root before reloading, and restores its previous fragments if verification fails. A setting before the `Include` line or a matching `Match` block can still override it; the effective-setting check will fail.

UFW rules carry a `pp-managed` comment. The playbook adds the desired rules first, then removes obsolete rules with that comment. It keeps both the configured SSH port and the current inventory connection port open. Switch the inventory to the new port after verifying access, then rerun to retire the old port. Existing rules without that comment remain under operator control; inspect and remove obsolete unmarked rules when adopting an existing host. Docker-published ports can bypass UFW, so bind application ports to loopback.

## Changing storage

Set `data_volume_backup_confirmed: true` only after checking the device and a usable backup, or confirming that the new device has no data to preserve. The role refuses a mount point that contains fallback data, another mounted device, old mount points to remove, or `nofail`. Stop consumers and move existing mounts deliberately before rerunning.

The host records the expected mounted device. Docker depends on that mount, and pp verifies it before deployment and service startup. If a database looks empty after boot, check `findmnt -M /data` before initializing anything. An unmounted directory can contain a separate database on the root filesystem.

For Docker relocation, set `docker_data_root_enabled: true`, `docker_data_root_path` and `docker_data_root_backup_confirmed: true`. The destination must be empty and must not overlap the current root. The helper stops running containers and Docker, copies from the root returned by `docker info`, and preserves that source. It merges daemon settings once, verifies the new root, then restarts the stopped containers. A failure restores the prior daemon config and restarts Docker against its original data.

`/etc/docker/daemon.json.pp-before` holds the previous daemon config. A killed provisioning client may require restoring it and starting Docker manually. Inspect a partial destination copy before retrying. Back up application databases separately, and include `/var/lib/containerd` when Docker uses its separate [containerd image store](https://docs.docker.com/engine/storage/containerd/). Disabling the storage role does not unmount data or remove its startup dependency.
