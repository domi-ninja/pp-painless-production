# 021 - Validate and reconcile SSH access and firewall rules

## Problem

Address audit findings 9, 10 with generic pp behavior and reusable examples.

## Work

- Reject placeholders and invalid SSH public keys before changes.
- Manage and revoke the declared authorized key set.
- Verify effective SSH configuration and order the managed fragment deliberately.
- Reconcile owned firewall rules while protecting SSH access.

## Acceptance

The listed behavior has focused checks and matching user documentation. Existing deployment identity, data paths and rollback artifacts remain usable.

## Decision

Should manual SSH keys remain outside the declared set?

Default: The playbook owns the admin account key set; document the replacement behavior.
