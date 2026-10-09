# 021 - Validate and reconcile SSH access and firewall rules

## Problem

SSH keys and firewall rules need reconciliation. Earlier sshd settings can override the intended access policy.

## Work

- Reject placeholders and invalid SSH public keys before changes.
- Manage and revoke the declared authorized key set.
- Verify effective SSH configuration and order the managed fragment deliberately.
- Reconcile owned firewall rules while protecting SSH access.

## Acceptance

Verify the behavior above with isolated fixtures and document its user-visible limits. Preserve existing deployment identity, data paths and recovery artifacts.

## Decision

Should manual SSH keys remain outside the declared set?

Default: The playbook owns the admin account key set; document the replacement behavior.
