# 011 - Report attempts and observed deployment state

## Problem

Status reports the last local success even when a later attempt leaves a failed or partial live deployment.

## Work

- Persist the latest attempt and each host's completed apply step.
- Record failed recovery and its checks separately.
- Report persistence failures instead of discarding them.
- Inspect current containers over SSH and compare their release labels and health with local metadata.
- Permit status without current secrets, Dockerfiles or Git history.

## Acceptance

Focused tests cover failed first deployment, partial apply, failed rollback, unreachable hosts and absent current build inputs.

## Decision

Should status contact the hosts by default?

Default: yes, with bounded SSH calls and explicit per-host errors; label local records separately from observed state.
