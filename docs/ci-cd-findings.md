# Codeberg CI/CD Self-Hosting Findings

Research date: 2026-06-10

## Short recommendation

For a Codeberg-hosted repository, the most practical "run CI/CD ourselves" path is usually:

1. Use **Forgejo Actions with a self-hosted runner** if we want CI results inside Codeberg's repository UI and GitHub-Actions-like workflow files.
2. Use **Codeberg Woodpecker CI plus our own Woodpecker agent** if we already have or prefer Woodpecker pipelines and only need to provide our own compute.
3. Run a **fully self-hosted Woodpecker server plus agents** only if we want our own CI control plane, webhook handling, UI, storage, secrets policy, and runner fleet.

I would start with Forgejo Actions self-hosted runner for a new setup unless existing repositories already use `.woodpecker/*.yml`. It has less infrastructure to expose publicly because the runner dials out to Codeberg and waits for jobs.

## Current Codeberg CI/CD landscape

Codeberg currently documents two CI/CD families:

- **Woodpecker CI**: Codeberg runs a hosted Woodpecker instance at `https://ci.codeberg.org`, but onboarding is manual and Codeberg warns the hosted service is best-effort, resource-limited, manually approved, and currently `linux/amd64` only. Source: https://docs.codeberg.org/ci/
- **Forgejo Actions**: Codeberg is built on Forgejo. Actions can use self-hosted runners; Codeberg's hosted Actions are still limited/open-alpha due to security and maintainership constraints. Source: https://docs.codeberg.org/ci/actions/

The important distinction:

- Forgejo Actions runners connect outbound to Codeberg. They do **not** need a public IP. Source: https://docs.codeberg.org/ci/actions/
- A full Woodpecker instance needs OAuth and webhook integration with Codeberg, which means the Woodpecker server normally needs a stable public URL reachable by browser callbacks and Codeberg webhooks.

## Option A: Forgejo Actions self-hosted runner

This is the closest equivalent to "GitHub Actions, but jobs run on our machine."

### What we host

- One or more `forgejo-runner` processes or containers.
- Optional Docker-in-Docker, LXC, or host Docker access depending on what workflows need.
- Persistent runner config/token storage.

### What Codeberg still provides

- Repository hosting.
- Actions UI and job result display.
- Registration token generation.
- Workflow dispatching.

### Setup outline

1. In the Codeberg repository settings, enable Actions under `Units > Overview`.
2. Obtain a runner token from the desired scope:
   - Repository: `/{owner}/{repo}/settings/actions/runners`
   - User: `/user/settings/actions/runners`
   - Organization: `/org/{org}/settings/actions/runners`
3. Run and register the runner against `https://codeberg.org/`.
4. Add workflows under `.forgejo/workflows/*.yaml`.
5. Use a `runs-on` label that matches the runner labels.

Minimal workflow example:

```yaml
on: [push]

jobs:
  test:
    runs-on: docker
    steps:
      - run: echo "All good"
```

Forgejo's quick start uses `.forgejo/workflows/demo.yaml` and `runs-on: docker`; if the runner has a different label, use that label instead. Source: https://forgejo.org/docs/latest/user/actions/quick-start/

### Docker Compose shape

Forgejo's runner install docs show an OCI-image setup using a `docker-in-docker` service plus a `data` volume for runner state. The initial command keeps the runner container alive for registration; after registration, replace it with `forgejo-runner daemon`. Source: https://forgejo.org/docs/v11.0/admin/actions/runner-installation/

Skeleton:

```yaml
services:
  docker-in-docker:
    image: docker:dind
    privileged: true
    command: ["dockerd", "-H", "tcp://0.0.0.0:2375", "--tls=false"]
    restart: unless-stopped

  runner:
    image: data.forgejo.org/forgejo/runner:4.0.0
    depends_on:
      - docker-in-docker
    environment:
      DOCKER_HOST: tcp://docker-in-docker:2375
    user: 1001:1001
    volumes:
      - ./data:/data
    restart: unless-stopped
    command: '/bin/sh -c "sleep 5; forgejo-runner daemon"'
```

Registration is interactive:

```sh
docker compose up -d
docker exec -it runner /bin/sh
forgejo-runner register
```

When prompted, use `https://codeberg.org/`, paste the runner token, name the runner, and set labels.

### Security notes

Forgejo Runner executes remote code, so the runner host should be treated as an untrusted execution environment. Source: https://forgejo.org/docs/latest/admin/actions/security/

For workflows that need Docker:

- Docker-in-Docker improves isolation from the host Docker daemon, but job containers may still see artifacts left in the DIND daemon and concurrent jobs can interact if they share the same DIND daemon.
- Mounting `/var/run/docker.sock` is simpler but exposes the host Docker daemon to jobs. Forgejo docs explicitly warn that this can allow workflows to inspect, mutate, or compromise host containers and storage.
- LXC gives stronger isolation for Docker-capable jobs, but has feature gaps such as unsupported service containers.

Source: https://forgejo.org/docs/latest/admin/actions/docker-access/

Security recommendations:

- Prefer repository-scoped runner tokens over user/org/global scope when possible.
- Use a dedicated VM or VPS per trust boundary.
- Avoid host Docker socket mounting for public or contributor-triggered workflows.
- Keep `runner.capacity` low unless jobs are trusted or isolated per job.
- Store deployment secrets in Codeberg/Forgejo Actions secrets, not in the runner image or repository.

## Option B: Codeberg hosted Woodpecker plus our own agent

This is a hybrid: Codeberg's Woodpecker server remains the control plane, but our machine supplies compute.

### What we host

- One or more Woodpecker agents.
- Docker, Kubernetes, or another supported backend.

### What Codeberg still provides

- Woodpecker server at `ci.codeberg.org`.
- Pipeline UI.
- Repo activation and webhook integration.

### Setup outline

1. Get access to Codeberg's Woodpecker CI if not already onboarded.
2. Create an agent token in Codeberg's Woodpecker UI for a user or organization.
3. Start an agent pointing at `grpc.ci.codeberg.org:443`.
4. Use Woodpecker labels to route workflows to our agents.

Codeberg's Docker Compose example:

```yaml
services:
  woodpecker-agent:
    image: woodpeckerci/woodpecker-agent:v3.13.0
    command: agent
    restart: always
    volumes:
      - woodpecker-agent-config:/etc/woodpecker
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - WOODPECKER_SERVER=grpc.ci.codeberg.org:443
      - WOODPECKER_AGENT_SECRET=${WOODPECKER_AGENT_SECRET}
      - WOODPECKER_GRPC_SECURE=true

volumes:
  woodpecker-agent-config:
```

Source: https://docs.codeberg.org/ci/agents/

Woodpecker agent labels are set with `WOODPECKER_AGENT_LABELS`, and workflows can request labels:

```yaml
labels:
  location: region-1
  peripheral: lora
```

Source: https://docs.codeberg.org/ci/agents/

### When this is best

- We already use Woodpecker syntax.
- We want less server administration than full Woodpecker.
- We need specialized hardware, ARM/RISC-V, long-running jobs, or more capacity than Codeberg's shared hosted runners.

### Downsides

- Still depends on Codeberg's hosted Woodpecker availability and onboarding.
- Woodpecker's RBAC is limited for larger/more complex team permission structures, according to Codeberg's caveats. Source: https://docs.codeberg.org/ci/
- The Codeberg-hosted Woodpecker default environment is volunteer-run and best-effort.

## Option C: Fully self-hosted Woodpecker connected to Codeberg

This is the "own CI/CD service" path.

### What we host

- Woodpecker server.
- Woodpecker agents.
- Database/storage. SQLite is default; Postgres or MariaDB is recommended for larger instances.
- Reverse proxy/TLS.
- OAuth app and webhook integration with Codeberg.

Woodpecker's architecture is server plus agent:

- The server provides UI/API, processes webhooks, and parses pipeline YAML.
- The agent executes workflows with Docker, Kubernetes, local, or other backends and connects to the server over gRPC.

Source: https://woodpecker-ci.org/docs/administration/general

### Setup outline

1. Pick a public CI hostname, for example `https://ci.example.com`.
2. Register an OAuth application on Codeberg/Forgejo with callback URL `https://ci.example.com/authorize`.
3. Deploy Woodpecker server and at least one agent.
4. Configure the Woodpecker Forgejo driver:
   - `WOODPECKER_FORGEJO=true`
   - `WOODPECKER_FORGEJO_URL=https://codeberg.org`
   - `WOODPECKER_FORGEJO_CLIENT=<oauth-client-id>`
   - `WOODPECKER_FORGEJO_SECRET=<oauth-client-secret>`
   - `WOODPECKER_HOST=https://ci.example.com`
   - `WOODPECKER_AGENT_SECRET=<random-shared-secret>`
5. Put the server behind TLS reverse proxy.
6. Log into Woodpecker, activate repositories, and let Woodpecker create webhooks. The user activating a repo needs admin rights on that repository.
7. Add `.woodpecker/*.yaml` pipeline files to repos.

Woodpecker requires an OAuth app with the forge, persistent server data under `/var/lib/woodpecker`, and a shared server/agent secret. Source: https://woodpecker-ci.org/docs/next/administration/installation/docker-compose

Forgejo-specific Woodpecker settings are documented here: https://woodpecker-ci.org/docs/next/administration/configuration/forges/forgejo

### Docker Compose shape

Adapted from Woodpecker's Docker Compose docs and Forgejo driver docs:

```yaml
services:
  woodpecker-server:
    image: woodpeckerci/woodpecker-server:v3
    restart: always
    ports:
      - "8000:8000"
      - "9000:9000"
    volumes:
      - woodpecker-server-data:/var/lib/woodpecker/
    environment:
      - WOODPECKER_OPEN=true
      - WOODPECKER_HOST=${WOODPECKER_HOST}
      - WOODPECKER_FORGEJO=true
      - WOODPECKER_FORGEJO_URL=https://codeberg.org
      - WOODPECKER_FORGEJO_CLIENT=${WOODPECKER_FORGEJO_CLIENT}
      - WOODPECKER_FORGEJO_SECRET=${WOODPECKER_FORGEJO_SECRET}
      - WOODPECKER_AGENT_SECRET=${WOODPECKER_AGENT_SECRET}

  woodpecker-agent:
    image: woodpeckerci/woodpecker-agent:v3
    command: agent
    restart: always
    depends_on:
      - woodpecker-server
    volumes:
      - woodpecker-agent-config:/etc/woodpecker
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - WOODPECKER_SERVER=woodpecker-server:9000
      - WOODPECKER_AGENT_SECRET=${WOODPECKER_AGENT_SECRET}

volumes:
  woodpecker-server-data:
  woodpecker-agent-config:
```

Generate the shared secret with:

```sh
openssl rand -hex 32
```

Woodpecker warns that there is no `latest` image tag; use a semver tag or major/minor rolling tag such as `v3`. Source: https://woodpecker-ci.org/docs/administration/general

### Pipeline example

Woodpecker workflows live under `.woodpecker/`. Example:

```yaml
when:
  - event: push
    branch: main

steps:
  - name: test
    image: golang:1.23
    commands:
      - go test ./...
```

Woodpecker activation requires admin rights on the repo because it creates a webhook. Source: https://woodpecker-ci.org/docs/usage/intro

### When this is best

- We want full control of runner fleet, UI, uptime, update cadence, storage, and secrets.
- We need CI for multiple Codeberg accounts/orgs without relying on Codeberg's hosted Woodpecker.
- We want Woodpecker plugins and YAML, not Forgejo Actions syntax.

### Downsides

- More operational surface: public endpoint, TLS, OAuth app, webhook delivery, database backups, updates.
- Separate UI from Codeberg's native Actions tab.
- More secret and runner isolation work lands on us.

## Comparison

| Path | Public endpoint needed | UI location | Workflow files | Best for |
| --- | --- | --- | --- | --- |
| Forgejo Actions self-hosted runner | No | Codeberg repo Actions tab | `.forgejo/workflows/*.yaml` | New setup, simple operations, GitHub-Actions-like workflows |
| Codeberg Woodpecker + self-hosted agent | No for agent; Codeberg hosts server | `ci.codeberg.org` | `.woodpecker/*.yaml` | Existing Woodpecker users, custom compute, less admin |
| Full self-hosted Woodpecker | Yes | Our Woodpecker URL | `.woodpecker/*.yaml` | Full control plane ownership |

## CD considerations

For deployment jobs, prefer these patterns:

- Keep deploy keys/tokens as CI secrets, not in repository files.
- Use repository-scoped runners for sensitive deployments.
- Separate "test untrusted PRs" runners from "deploy to production" runners.
- Restrict deploy workflows to protected branches or tags.
- For Codeberg Pages/static deployments, Codeberg has docs for deploying from Forgejo Actions, but the same general principles apply: protect credentials and restrict write/publish jobs to trusted refs.

## Implementation plan I would use

Phase 1: Proof of concept with Forgejo Actions

1. Create a small VPS or VM dedicated to CI.
2. Install Docker and Docker Compose.
3. Create a repository-scoped Codeberg runner token.
4. Start `forgejo-runner` with Docker-in-Docker.
5. Register the runner to `https://codeberg.org/`.
6. Commit `.forgejo/workflows/smoke.yaml`.
7. Confirm the job appears and passes in Codeberg's Actions tab.

Phase 2: Production hardening

1. Move runners to dedicated VMs per trust boundary.
2. Avoid host Docker socket mounting unless workflows are fully trusted.
3. Store secrets in Codeberg Actions secrets.
4. Add branch/tag restrictions for deploy jobs.
5. Add backups for runner config and document token rotation.
6. Add monitoring for disk growth, especially Docker/DIND image caches.

Phase 3: Decide if Woodpecker is needed

Use Woodpecker instead if:

- Existing repos already have `.woodpecker` pipelines.
- We need Woodpecker plugins/features.
- We want a CI service independent from Codeberg Actions alpha status.
- We need central CI fleet administration across multiple repos/orgs.

## Source list

- Codeberg CI overview: https://docs.codeberg.org/ci/
- Codeberg Forgejo Actions self-hosted runner guide: https://docs.codeberg.org/ci/actions/
- Codeberg self-hosted Woodpecker agents: https://docs.codeberg.org/ci/agents/
- Forgejo Actions admin guide: https://forgejo.org/docs/latest/admin/actions/
- Forgejo Actions security guide: https://forgejo.org/docs/latest/admin/actions/security/
- Forgejo Runner installation: https://forgejo.org/docs/v11.0/admin/actions/runner-installation/
- Forgejo Docker access for Actions: https://forgejo.org/docs/latest/admin/actions/docker-access/
- Forgejo Actions user guide: https://forgejo.org/docs/latest/user/actions/overview/
- Forgejo Actions quick start: https://forgejo.org/docs/latest/user/actions/quick-start/
- Woodpecker general architecture: https://woodpecker-ci.org/docs/administration/general
- Woodpecker Docker Compose install: https://woodpecker-ci.org/docs/next/administration/installation/docker-compose
- Woodpecker Forgejo integration: https://woodpecker-ci.org/docs/next/administration/configuration/forges/forgejo
- Woodpecker first pipeline: https://woodpecker-ci.org/docs/usage/intro
