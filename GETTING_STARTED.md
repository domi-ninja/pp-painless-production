# Getting started

Keep two repositories separate: this tool's clone supplies `pp` and the server playbook; your application's repository supplies its Dockerfile and `deploy.yml`.

## 1. Clone and install

On your deployment machine, you need Git, Go 1.23 or newer, Make, SSH/SCP and a working Docker daemon for local image builds. Install Ansible there too if you will provision servers. The CLI uses Unix file locking, so use a Linux or macOS environment.

```sh
git clone https://github.com/domi-ninja/pp-painless-production.git
cd pp-painless-production
go test ./...
make install
```

`make install` runs `go build` and installs the binary as `pp` in `GOBIN`, or in `$(go env GOPATH)/bin` when `GOBIN` is unset. Add that directory to your shell's `PATH`, then check:

```sh
command -v pp
pp --help
```

You can choose a directory already on your PATH with `make install PP_BIN=/your/bin/pp`. Plain `go install ./cmd/deploy` would name the binary `deploy`, not `pp`.

Keep the full clone. To update, pull the source, review the changes, run the tests and repeat `make install`. Installing `pp` does not provision any server.

## 2. Prepare a host, usually once

Skip this step if your host already has Docker Engine, the Compose plugin, SSH access and the permissions below. Public HTTPS routes also need host-level Caddy and inbound ports 80/443.

For a new Ubuntu 22.04+ host, work from the tool's clone:

```sh
cd infra/ansible
cp inventory/prod.example.yml inventory/prod.local.yml
```

Edit the copied inventory with your server IP and bootstrap SSH user. Create `group_vars/bootstrap.local.yml` with your real public key and email:

```yaml
admin_user: deploy
admin_ssh_public_keys:
  - "ssh-ed25519 YOUR_PUBLIC_KEY"
docker_users:
  - deploy
reverse_proxy_enabled: true
reverse_proxy_email: you@example.com
```

Both `.local.yml` files are ignored by Git. Keep credentials in ignored files or Ansible Vault. Review the [provisioning settings](infra/ansible/README.md), especially SSH, firewall and storage changes, before applying them. The defaults disable root and password SSH login; keep your bootstrap session open until you have verified a fresh login as `deploy`.

```sh
ansible-playbook -i inventory/prod.local.yml \
  -e @group_vars/prod_servers.yml \
  -e @group_vars/bootstrap.local.yml \
  playbooks/prod-server.yml
```

This explicitly loads the baseline settings and your local overrides. Leave Forgejo, CI runners and storage relocation disabled unless needed. For later provisioning runs, update the inventory to use the configured admin user instead of root.

Verify a fresh SSH connection and Docker access:

```sh
ssh deploy@YOUR_SERVER 'docker info >/dev/null && docker compose version'
```

The SSH account also needs to write release files under its home directory, manage `/etc/pp/ports.tsv` and `/etc/pp/proxy/routes/`, and reload Caddy. The current CLI does not invoke `sudo` automatically. The playbook creates root-owned proxy directories, so configure and verify these permissions for your deployment account before using `pp`. Passwordless sudo alone does not make those direct commands work.

Point your application's DNS records at the host. Caddy handles public HTTPS while container ports stay on `127.0.0.1`.

## 3. Configure and deploy an app

Switch to your application's Git repository, not the tool's clone:

```sh
cd /path/to/your-app
pp init
```

Skip `init` if the app already has `deploy.yml`. Edit the starter's SSH target, domain and container port, and replace its `true` health check with a real check. It assumes a Dockerfile and port 3000. See the [simple website example](examples/simple-website/) or [other examples](examples/).

Commit `deploy.yml`. Keep secrets out of Git and the Docker build context. `pp init` adds `.deploy/` to `.gitignore` and `.deploy` to `.dockerignore`; add your secret files separately.

```sh
pp plan
pp
pp status
```

`plan` does not deploy, though it can upgrade local state. Review the selected host and environment before running `pp`, which performs the deployment. Config paths are relative to the application's working directory.

Preserve `.deploy/`: it contains local state and rollback artifacts. See the [version migration guide](deploy-cli.md#migrating-an-existing-deployment) and [security review](SECURITY_REVIEW.md) before trusting this experimental tool with production secrets.

From here, ordinary app releases only need `pp`. Return to Ansible when the server itself needs a configuration change.
