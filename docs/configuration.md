# Configuration

pp reads one YAML document with strict fields. Unknown keys, unsupported phases, invalid references and missing required env values fail before deployment. Config version is 1; omission also means 1. Paths in the config are relative to the app's working directory, not the config file's directory. Repo inputs must stay inside that checkout.

Project, environment, host, build, service and volume names use lowercase letters, digits and hyphens, beginning and ending with a letter or digit.

## Project and hosts

```yaml
version: 1
project:
  name: shop
  environment: prod
hosts:
  server:
    ssh: deploy@example.com
    roles: [web]
```

Every service declares at least one existing host. `ssh` is a hostname or `user@hostname`. Put ports, jump hosts and keys in SSH config. Roles are descriptive labels, not scheduling or security rules. CI and Git-hosting roles are refused on application hosts.

## Builds

Use `build` for one image or `builds` for named images. Prefer named builds for multiple services.

```yaml
builds:
  web:
    context: .
    dockerfile: Dockerfile
    target: runtime
    platforms: [linux/amd64]
    tags: ["shop-web:${release}"]
    args:
      SITE_URL: https://shop.example.com
```

`context` must exist as a directory; `dockerfile` must be a regular file. Both stay inside the checkout. `target`, `platforms`, `tags` and `args` are optional. Each build supports one platform, `linux/amd64` or `linux/arm64`; omission defaults to `linux/amd64`. pp loads images into the local daemon, so it does not export a multi-platform index.

An omitted tag in a named build becomes `PROJECT-BUILD:${git_sha}`; the legacy single build uses `PROJECT:${git_sha}`. Prefer `${release}` so distinct deploys of the same commit do not share a mutable app tag. A built service's image must match a tag of its selected build and cannot use an upstream pull policy. A single legacy build can be inferred when the service image matches its tag.

Upstream-only deployments omit both build forms. Build arguments are public configuration and can remain in image history. Never pass credentials through them.

## Environment files

The same selection fields apply to services, hooks, command checks and migrations:

```yaml
env:
  source: .env.prod
  required: [DATABASE_URL, JWT_SECRET]
```

`required` selects the keys and requires nonempty values. A source without a selection requires `include_all: true`. Top-level `env` supplies template inputs, not automatic service injection. Child processes still inherit ordinary operator settings; hooks are not a sandbox.

Env files contain one `KEY=VALUE` per line. Keys use ASCII letters, digits and underscores and cannot begin with a digit. Blank lines and lines beginning with `#` are ignored. One matching pair of outer quotes is removed. Other quotes, dollar signs, backslashes and inline `#` characters stay literal. Duplicate keys, `export`, unmatched outer quotes and multiline values fail.

## Services

```yaml
services:
  web:
    image: shop-web:${release}
    build: web
    phase: frontend
    hosts: [server]
    env:
      source: .env.prod
      required: [JWT_SECRET]
    environment:
      PORT: "3000"
    ports:
      - published: auto
        target: 3000
    health:
      http: http://127.0.0.1:3000/health
      timeout_seconds: 60
```

`image` and `hosts` are required. `build` selects a named build or `default` for the legacy build. Upstream `pull` accepts `if_missing`, `always` or `never`; omission means no pull. `phase` accepts `infra`, `backend` or `frontend`; omission joins the final apply.

`command` and `entrypoint` are argument arrays, not shell strings. `environment` supplies inline runtime values. `command_env` is another inline environment map and takes precedence over matching `environment` keys. Inline environment overrides the selected env file. Put credentials in protected runtime env files.

Every exposed or routed service needs a health check. Choose `health.http` or `health.command`, not both. HTTP health runs wget inside the container; include that tool or use a command the image provides. Commands run directly. Compose checks every 10 seconds with a 5 second attempt timeout; `health.timeout_seconds` controls the retry count, default 60 seconds. Final Compose waiting has a separate 300 second bound.

Ports use `published: auto` or a number from 1 through 65535, plus a numeric `target` in that range. `host_ip` defaults to `127.0.0.1` and must be an IP address when set. See [ports](deploy-cli.md#ports-and-routes) for allocation and retirement.

## Volumes

```yaml
volumes:
  data:
    driver: local
    external: false
services:
  web:
    volumes:
      - name: data
        target: /data
      - source: /srv/shop/config
        target: /etc/shop
        type: bind
        read_only: true
```

Each mount has exactly one of `name` or `source`, an absolute container `target`, optional `type` and optional `read_only`. `name` defaults to a named volume and must appear in top-level `volumes`. `source` defaults to a bind and must be an absolute host path. Binds use `create_host_path: false`; prepare their directories and ownership before deployment. A typo must not create an empty database directory.

Top-level volume fields are `driver` and `external`. Compose scopes managed volumes by deployment; external volumes and bind paths need operator-managed isolation. Down and cleanup never delete application volumes.

## Routes

```yaml
routes:
  - host: shop.example.com
    service: web
    target_port: 3000
    host_id: server
route_files:
  - source: deploy/shop.caddy.tmpl
    host: server
```

A generated route needs `host` and `service`. It can also declare an explicit upstream `target`, such as `http://127.0.0.1:8080`. `target_port` selects the service port; omission requires exactly one service port. `host_id` selects a host containing that service; omission renders the route on every host containing it. Hostnames and upstream origins are validated; credentials and URL paths are not accepted in explicit upstream origins.

Route-file sources stay inside the checkout. Files for one host are combined into one owned Caddy file. There is no `dest_name`; remove it from older configs. Templates can use public env inputs and resolved service ports. They are trusted Caddy configuration.

## Hooks and checks

Hooks are ordered arrays under `after_infra_healthy`, `after_backend_healthy` or `after_services_healthy`. Each has a required `name` and `run` array, optional `env`, and `timeout_seconds`, default 300. pp supplies reserved `PP_PROJECT`, `PP_ENVIRONMENT`, `PP_CONFIG` and `PP_ENV_FILE`; `PP_HOST` is available for a single-host deployment. Helpers must select both project and environment.

Only `checks.smoke` is supported. Each check has `name` and either `url` or `command`. HTTP options are `expect_status`, default 200; `follow_redirects`, default true; `attempts`, default 3 and maximum 20; and `timeout_seconds`, default 15 for the whole retry window. HTTP retries wait one second between attempts. Command checks support `env` and `timeout_seconds`, run once, and cannot request multiple attempts. URLs and arguments must not contain credentials.

## Migrations

```yaml
migrations:
  image: shop-migrations:${release}
  command: [migrate, up]
  rollback_command: [migrate, down, "1"]
  env:
    source: .env.prod
    required: [DATABASE_URL]
  run: before_services
  timeout_seconds: 300
```

`image` and `command` are required. Specify `rollback_command` or a written `restore_plan` describing manual recovery. `run` accepts only `before_services`, which is also the default. The migration container runs on the deployment machine; it needs network access to its database. pp does not mount app volumes into it. Choose migration images available locally or reachable through their declared source.

Migration arguments and selected inputs are saved before execution. Container rollback cannot undo arbitrary data changes. See [recovery](recovery.md) and the [backup guide](../examples/backups.md).

## Templates and retention

Templates support `${project}`, `${environment}`, `${git_sha}`, `${short_sha}`, `${release}`, `${env.KEY}` and `${service.NAME.port.TARGET}`. They apply to images, tags, build args, runtime values, commands, health, mounts and routes. Hooks, checks and migration arguments use their declared env inputs. Missing pp inputs fail validation. Replacements happen once; dollar expressions inside an env value remain literal.

`retention` supports `releases`, `failed`, `min_free_mb` and `build_cache_mb`. Defaults and cleanup limits are described in [retention](deploy-cli.md#retention-and-cleanup). All configured timeouts accept zero for the default or 1 through 86400 seconds.
