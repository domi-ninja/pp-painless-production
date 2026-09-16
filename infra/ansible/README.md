# Ubuntu production server provisioning

This Ansible stack provisions an Ubuntu 22.04+ host with a practical production baseline:

- Non-root sudo admin user with SSH keys.
- SSH hardening with password login disabled by default.
- UFW firewall with default-deny inbound policy, fail2ban, unattended security updates, chrony time sync, logrotate, journald retention, and basic sysctl hardening.
- Docker Engine, the Docker Compose plugin, and a `docker-compose` compatibility command.
- Optional persistent data volume mounted at `/data`.
- Optional Forgejo install using Docker Compose under `/data/forgejo`.
- Optional Caddy reverse proxy for Forgejo and `pp`-managed application routes.

CI runners use the separate `runner-server.yml` playbook on dedicated VMs. They are refused on production application hosts.

## Layout

```text
infra/ansible/
  ansible.cfg
  inventory/prod.example.yml
  group_vars/prod_servers.yml
  playbooks/prod-server.yml
  roles/
```

## First run

Install Ansible on your workstation, then create a local inventory from the example. The playbook uses Ansible core modules only.

```sh
cd infra/ansible
cp inventory/prod.example.yml inventory/prod.local.yml
cp host_vars/prod.example.yml host_vars/prod-1.local.yml
```

Edit `inventory/prod.local.yml` with the server IP and bootstrap SSH user. Set your public key and email in `host_vars/prod-1.local.yml`, copied from the [host-vars example](host_vars/prod.example.yml). Keep the example unchanged and real values in the ignored copy.

Run the playbook:

```sh
ansible-playbook -i inventory/prod.local.yml --limit prod-1 \
  -e @group_vars/prod_servers.yml \
  -e @host_vars/prod-1.local.yml \
  playbooks/prod-server.yml
```

The `.local.yml` file is loaded explicitly, not discovered by hostname. Replace `prod-1` with your inventory hostname and keep `--limit` so the overrides cannot affect other hosts. See [Getting started](../../GETTING_STARTED.md#2-prepare-a-host-usually-once) for SSH lockout precautions and the additional permissions `pp` needs.

After the first run, connect as the configured admin user:

```sh
ssh deploy@SERVER_IP
```

## Important variables

| Variable | Default | Purpose |
| --- | --- | --- |
| `admin_user` | `deploy` | Non-root sudo user to create. |
| `admin_ssh_public_keys` | placeholder | Public keys installed for `admin_user`. |
| `ssh_password_authentication` | `false` | Disables password SSH when keys are configured. |
| `ufw_allowed_tcp_ports` | `22`, `80`, `443` | TCP ports opened in UFW. |
| `swap_file_size_mb` | `0` | Set to a positive number to create a swap file. |
| `docker_users` | `deploy` | Users added to the `docker` group. |
| `data_volume_enabled` | `false` | Mounts an attached block volume, normally at `/data`. |
| `docker_data_root_enabled` | `false` | Moves Docker's data root to `docker_data_root_path`, normally `/data/docker`. |
| `coolify_enabled` | `false` | Installs and starts Coolify from `/data/coolify/source`. |
| `reverse_proxy_enabled` | `false` | Installs Caddy and imports route files from `/etc/pp/proxy/routes`. |
| `forgejo_enabled` | `false` | Installs and starts Forgejo from `/data/forgejo`. |

## Coolify

Enable Coolify in an ignored inventory or host vars file:

```yaml
data_volume_enabled: true
data_volume_src: /dev/disk/by-id/scsi-0HC_Volume_EXAMPLE
data_volume_path: /data
data_volume_previous_mountpoints:
  - /mnt/HC_Volume_EXAMPLE

docker_data_root_enabled: true
docker_data_root_path: /data/docker

coolify_enabled: true
coolify_app_url: "https://coolify.example.com"
coolify_app_port: "8000"
coolify_proxy_ui_enabled: true
coolify_proxy_hostname: coolify.example.com
coolify_default_project_enabled: true
coolify_default_project_name: Default
```

Put first-admin credentials in `infra/ansible/host_vars/<host>.credentials.local.yml`, which is ignored:

```yaml
coolify_admin_create_enabled: true
coolify_admin_name: "Admin"
coolify_admin_email: "admin@example.com"
coolify_admin_password: "use-a-generated-secret"
```

The Coolify role stores persistent data in `/data/coolify`, keeps Docker state in `/data/docker` when `docker_data_root_enabled` is true, and installs a Traefik dynamic route so the Coolify UI is reachable through the Coolify proxy on ports 80/443.

Existing Coolify application records can be kept aligned with local vars:

```yaml
coolify_application_overrides:
  - uuid: hgtso9vjh2pf9lmp916mgta6
    git_repository: "https://git.example.com/owner/repo.git"
    git_branch: main
    fqdn: "https://app.example.com"
```

For `p3.domi.ninja`, the real deployment values are kept in ignored local files:

- `infra/ansible/inventory/prod.local.yml`
- `infra/ansible/host_vars/p3.yml`

## Forgejo

Enable Forgejo in ignored host vars:

```yaml
reverse_proxy_enabled: true
reverse_proxy_email: "admin@example.com"

ufw_allowed_tcp_ports:
  - 22
  - 80
  - 443
  - 2222

forgejo_enabled: true
forgejo_domain: git.example.com
forgejo_root_url: "https://git.example.com/"
forgejo_ssh_port: "2222"
forgejo_admin_create_enabled: true
forgejo_admin_username: "admin"
forgejo_admin_email: "admin@example.com"
```

Keep `forgejo_db_password`, `forgejo_secret_key`, `forgejo_internal_token`, `forgejo_lfs_jwt_secret`, `forgejo_oauth2_jwt_secret`, and `forgejo_admin_password` in an ignored credentials file. The Forgejo role stores application and database data under `/data/forgejo`, publishes a Caddy route under `/etc/pp/proxy/routes/forgejo.caddy` when `reverse_proxy_enabled` is true, and exposes Git SSH on the configured high port.

For legacy hosts still using a Coolify/Traefik proxy network, explicitly set `forgejo_proxy_dynamic_dir` and `forgejo_proxy_network`. New hosts should use `reverse_proxy_enabled: true` instead.

## Forgejo Actions runner

Runners are refused by `prod-server.yml`. Use a separate VM in inventory group `runner_servers`, set `ci_runner_isolated_host: true`, and run `playbooks/runner-server.yml`. Provide that VM's SSH key, `docker_users` and other baseline variables as local overrides. Do not put it in `prod_servers` or host application containers there. Treat workflows as root-equivalent access to the runner VM. Restrict runner registration and accepted jobs to trusted repositories and refs.

Forgejo's Docker-in-Docker socket is shared only with its runner over a Unix socket. It no longer listens on unauthenticated TCP port 2375. Privileged DinD still requires VM isolation.

Enable the optional runner:

```yaml
forgejo_runner_enabled: true
forgejo_runner_registered: false
forgejo_runner_name: codeberg-prod-1
```

Run `runner-server.yml` against the dedicated VM. It starts the runner container in a waiting state so you can register it without storing a token in Git:

```sh
ssh deploy@SERVER_IP
cd /opt/forgejo-runner
sudo docker compose exec runner /bin/sh
forgejo-runner register --no-interactive \
  --instance https://codeberg.org/ \
  --name codeberg-prod-1 \
  --token RUNNER_TOKEN \
  --labels "docker:docker://node:22-bookworm,ubuntu-24.04:docker://ubuntu:24.04"
exit
```

Then set `forgejo_runner_registered: true` and rerun the playbook to start the runner daemon.

## Woodpecker agent

The same dedicated-VM policy applies. The host Docker socket grants control of that runner VM; it must not be a production application host.

Enable the optional Codeberg-hosted Woodpecker agent:

```yaml
woodpecker_agent_enabled: true
woodpecker_agent_secret: "use-ansible-vault-or-an-ignored-vars-file"
woodpecker_agent_labels:
  location: prod-1
```

The Woodpecker agent uses `/var/run/docker.sock`. Only use this on hosts where the pipelines are trusted.

Runner installation writes `/etc/pp/ci-runner-host`; `pp` refuses application deployments to marked hosts. Removing runners does not remove that safety marker automatically. Existing shared hosts must be split manually before adopting this policy.

## Notes

- Keep real secrets in Ansible Vault or ignored local vars files.
- Running CI jobs executes remote code. Use dedicated hosts or VMs per trust boundary.
- If you change `ssh_port`, update `ufw_allowed_tcp_ports` at the same time.
