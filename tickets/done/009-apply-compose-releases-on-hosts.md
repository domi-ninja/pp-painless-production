# 009 - Apply Compose releases on hosts

## Goal

Apply uploaded compose bundles on each host and record release state.

## Dependencies

- 006
- 008

## Scope

- Run host-local `docker compose up -d` from the uploaded release bundle.
- Use deterministic compose project names.
- Set strict permissions on runtime env provisioning artifacts.
- Query observed host state through SSH before and after apply.
- Mark release status as applying, applied, failed, or verified.
- If a later host fails, identify which hosts already applied and need rollback.

## Deliverables

- Apply module.
- Remote command runner.
- Deployment metadata writer.
- Failure handling path that preserves logs.

## Acceptance

- Apply fails if SSH preflight fails.
- Apply does not proceed if required image tags are missing.
- Host state records release ID, git SHA, services, status, and log path.
- A failed host apply returns enough context for automatic rollback of already-updated hosts.
