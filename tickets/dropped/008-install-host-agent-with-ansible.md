# 008 - Install host agent with Ansible

## Goal

Install and manage the Go host agent on prod servers using Ansible.

## Dependencies

- 007

## Scope

- Add or extend Ansible role for the host agent.
- Install the agent binary.
- Create user, directories, config, and systemd unit.
- Configure localhost binding for SSH-reached agent access.
- Add agent host ID and auth config to inventory/group vars.

## Deliverables

- Ansible role/tasks/templates/defaults.
- Example inventory values.
- Handler for service restart.
- README for provisioning and upgrade.

## Acceptance

- Running the playbook installs and starts the agent.
- Re-running the playbook is idempotent.
- Agent state directory survives binary upgrades.
- CLI can reach the agent through SSH using configured host metadata.
