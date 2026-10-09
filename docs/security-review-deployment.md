# Deployment security

pp runs trusted application code and configuration with the deployment operator's permissions. Docker access grants control of the host. Project and environment identities prevent accidental overlap; they do not isolate hostile tenants.

## Secrets

Keep secrets in ignored env files and outside Docker build contexts. `env.required` selects the keys passed to services, hooks, checks and migrations. Whole-file injection requires `include_all: true`. Child processes inherit ordinary operator tool settings.

Build arguments and Caddy route templates are public configuration. Use runtime env files for secrets. Command arguments and check URLs must not contain credentials. pp omits argv and URL queries from its own diagnostics, but application scripts still control their output.

Release directories are private. Old releases contain old runtime secrets until retention removes them. Preserve current and previous releases for recovery, and protect backups of `.deploy` as secret-bearing data.

## Host access

Only trusted operators and deployment jobs should have the SSH key and Docker permissions. Run untrusted CI work on separate hosts. pp refuses a host marked with `/etc/pp/ci-runner-host`; the application playbook rejects CI role flags.

Published container ports default to loopback. A public binding needs an explicit Docker-aware firewall or upstream firewall policy. UFW alone does not restrict Docker-published ports. See [Docker firewall behavior](https://docs.docker.com/engine/network/packet-filtering-firewalls/#docker-and-ufw).

Generated routes validate hostnames and upstream origins. Custom route templates and shell hooks are trusted code. Config and artifact paths stay inside the application checkout; state and ignore-file writes reject symlink destinations.

## Updates and verification

Use a supported, patched Go compiler, starting with Go 1.26.9. Rebuild pp after compiler updates. Check source dependencies with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`. Check the installed binary's compiler with `go version -m "$(command -v pp)"`.

Review infrastructure image updates separately from app releases. Image pinning makes a recorded release repeatable; it does not check for new vulnerabilities. Keep data backups and test restoration before a database or storage upgrade.

Database rollback and application hook recovery require explicit policies. Read the recovery instructions before depending on a release rollback for data restoration.
