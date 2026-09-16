# Getting started

Keep two repositories separate: this tool's clone supplies `pp` and the server playbook; your application's repository supplies its Dockerfile and `deploy.yml`.

## 1. Clone and install

On your deployment machine, you need Git, Go 1.23 or newer, Make, SSH/SCP and a working Docker daemon with Buildx for local image builds. Buildx must support the cache-pruning flags listed under [retention](deploy-cli.md#retention-and-cleanup). pp creates its own builder and downloads BuildKit on first use. Install Ansible there too if you will provision servers. The CLI uses Unix file locking, so use a Linux or macOS environment.

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

Skip this step if your host already has Docker Engine, Compose 2.30+ with raw env-file support, SSH access and the permissions below. Public HTTPS routes also need host-level Caddy and inbound ports 80/443.

For a new Ubuntu 22.04+ host, work from the tool's clone:

```sh
cd infra/ansible
cp inventory/prod.example.yml inventory/prod.local.yml
cp host_vars/prod.example.yml host_vars/prod-1.local.yml
```

Edit the copied inventory with your server IP and bootstrap SSH user. Edit the [host-vars example](../infra/ansible/host_vars/prod.example.yml) copy with your real public key and email. It enables Docker and Caddy while leaving storage changes and platform services off.

Both `.local.yml` files are ignored by Git. Keep credentials in ignored files or Ansible Vault. Review the [provisioning settings](../infra/ansible/README.md), especially SSH, firewall and storage changes, before applying them. The defaults disable root and password SSH login; keep your bootstrap session open until you have verified a fresh login as `deploy`.

```sh
ansible-playbook -i inventory/prod.local.yml --limit prod-1 \
  -e @group_vars/prod_servers.yml \
  -e @host_vars/prod-1.local.yml \
  playbooks/prod-server.yml
```

This explicitly loads the baseline settings and your local overrides; the `.local.yml` filename is not automatically associated with a host. Replace `prod-1` with your inventory hostname in both the filename and `--limit`. Leave storage relocation disabled unless needed. Git hosting and CI provisioning live in [git-host-ansible](https://git.domi.ninja/domi-ninja/git-host-ansible); CI runners require separate hosts. For later provisioning runs, update the inventory to use the configured admin user instead of root.

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

Skip `init` if the app already has `deploy.yml`. Edit the starter's SSH target, domain and container port, and replace its `true` health check with a real check. It assumes a Dockerfile and port 3000.

For a fuller starting point, choose a sample instead of the generated starter:

- [Simple website](../examples/simple-website/README.md): a pnpm frontend built into an nginx container, with a health check and HTTPS route.
- [Stateful Go website](../examples/stateful-go-website/README.md): a Go app with SQLite migrations, a persistent volume and a required secret.
- [Convex website](../examples/convex-website/README.md): a frontend, Convex backend and dashboard, Postgres and MinIO, with setup and deployment hooks.

These are deployment templates, not complete runnable apps. Read the sample's README for its expected application layout. Copy the needed files into your app, merging ignore rules and adapting any existing Dockerfile or scripts. Replace the sample project name, SSH target and domains, match the build output and container port to your app, and create any required env file with your own secrets.

Commit `deploy.yml`. Keep secrets out of Git and the Docker build context. `pp init` adds `.deploy/` to `.gitignore` and `.deploy` to `.dockerignore`; add your secret files separately.

```sh
pp plan
pp
pp status
```

`plan` does not deploy, though it can upgrade local state. Review the selected host and environment before running `pp`, which performs the deployment. Config paths are relative to the application's working directory.

Preserve `.deploy/`: it contains local state and rollback artifacts. See the [version migration guide](deploy-cli.md#migrating-an-existing-deployment) and [deployment security review](security-review-deployment.md) before trusting this experimental tool with production secrets.

pp automatically keeps three successful releases and one failed attempt, while protecting current and previous releases. Run `pp cleanup --dry-run` to preview expired bundles and image references. For CI, reuse the checkout and its `.deploy/` directory between jobs. See [retention and cleanup](deploy-cli.md#retention-and-cleanup) for limits and retry behavior.

From here, ordinary app releases only need `pp`. Return to Ansible when the server itself needs a configuration change.
