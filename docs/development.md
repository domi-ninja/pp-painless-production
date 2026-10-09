# Development

The Go CLI lives in `cmd/deploy`, with command handling in `internal/cli` and deployment behavior in `internal/deploy`. Ansible prepares application hosts under `infra/ansible`. Examples supply app-owned deployment files.

Use the compiler from `go.mod` or a newer supported, patched release:

```sh
make build
bin/pp version
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

`make build` writes `bin/pp`. `make install` explicitly replaces the installed binary; `make clean` removes only `bin/pp`. Inspect an installed binary with `go version -m "$(command -v pp)"` after upgrading the compiler.

Tests use repo-owned temporary fixtures and mocked transport. When Docker is available, the Compose interpolation test imports a disposable fixture image and inspects container metadata without starting it. They do not contact production hosts or sibling apps. Do not make live credentials or infrastructure a test prerequisite.

## Helper and host checks

Use Node 22 for the Convex helpers and Python 3.12+ for host tests. Install Ansible dependencies in a virtual environment:

```sh
python3 -m venv .venv
. .venv/bin/activate
python3 -m pip install -r infra/ansible/requirements.txt
cd infra/ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook -i inventory/prod.example.yml playbooks/prod-server.yml --syntax-check
cd ../..
PYTHONDONTWRITEBYTECODE=1 python3 infra/ansible/tests/test_host_security.py
PYTHONDONTWRITEBYTECODE=1 python3 infra/ansible/tests/test_access.py
PYTHONDONTWRITEBYTECODE=1 python3 infra/ansible/tests/test_storage.py
node --test examples/convex-website/scripts/bootstrap.test.cjs
```

The access tests need `ssh-keygen` and `sshd`. They run key and effective-policy checks against temporary files, not the host's SSH configuration. Storage and firewall tests mock destructive commands. `ansible.posix` supplies mount handling. CI installs these dependencies and runs the same checks plus shell and Node syntax checks.

## Isolated Docker tests

Retention and migration container checks are opt-in. The migration test needs the pinned Debian image:

```sh
docker pull debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
PP_TEST_DOCKER=1 go test ./internal/deploy -run 'Docker|Retention' -count=1
```

Read the test before enabling Docker access. Tests must name and clean up their own containers, networks, volumes and images. Never run a global prune as test cleanup.

To validate the Convex storage template:

```sh
docker build -t pp-audit-storage:seaweed examples/convex-website/deploy/storage
PP_TEST_DOCKER=1 python3 examples/convex-website/tests/storage_integration.py
```

This tests authenticated reads and writes, multipart uploads, denial of foreign-bucket and anonymous access, internal API isolation and persistence across restart. It uses a disposable store, not an existing data directory. It does not prove that every Convex feature or a particular app works. Test function deployment, file upload, export/import and a restored database in an isolated copy of the real app before upgrading its storage.

## Formats and reviews

Config and state versions advance independently. Add an explicit migration when a state change requires one. Preserve existing resource identities and recovery history. The [configuration reference](configuration.md) describes implemented fields; update it when behavior changes.

Use one numbered ticket and one implementation commit per completed change. Keep app-specific operational records and agent notes outside this repository. Before merging, review migration ordering, saved secret inputs, ownership checks, destructive cleanup and host access changes. Compiler vulnerability checks cover pp; scan built application and infrastructure images separately.
