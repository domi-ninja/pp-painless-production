# Configure pp hosts with Terraform

The host module runs pp's existing Ansible setup and installs Beszel monitoring.
It accepts a reachable server address, so a caller can pass the address returned
by its cloud VM resource. This module configures the machine; it does not purchase
a VM or choose a cloud provider.

## Use the module

The Terraform runner needs Node.js 22.18+, Ansible with the dependencies from
`infra/ansible/requirements.txt`, and the server's SSH key in its agent or SSH
configuration. Verify the server's host key before provisioning. Keep the whole
pp checkout because the module calls its Ansible playbooks.

Install the local `terraform.local/domi/beszel` provider from the infra repo's
`providers/beszel` directory into your Terraform root's plugin mirror. It is not
published in the Terraform Registry. Configure its hub URL and maintainer login
in the calling root module; credentials never go into the host's agent config.

```hcl
provider "beszel" {
  url      = var.beszel_url
  email    = var.beszel_email
  password = var.beszel_password
}

module "host" {
  source   = "/path/to/pp/infra/terraform/modules/host"
  name     = "application-server"
  address  = var.server_address
  ssh_user = "root"
  host_variables_json = jsonencode({
    admin_user            = "deploy"
    admin_ssh_public_keys = [var.deploy_public_key]
    docker_enabled        = true
    reverse_proxy_enabled = true
    reverse_proxy_email   = var.operator_email
  })
}
```

Run `terraform init`, review `terraform plan`, then `terraform apply`. The module
registers the host in Beszel, creates its alert rules, and provisions the host.
After confirming access as the configured admin user, change `ssh_user` to that
user. To adopt an already configured host, set `configure_base = false`; only the
monitoring role runs. `local_connection = true` configures the Terraform runner
itself and requires appropriate local privileges.

## Automatic monitoring

The agent discovers Docker containers through the local Docker socket. No
container list or per-container Terraform resources are needed. A startup helper
discovers mounted ext, XFS, Btrfs, and ZFS filesystems, omitting duplicate mounts;
Beszel monitors the root filesystem separately. Restart the agent after mounting
a new data disk so it refreshes discovery.

Every host gets these defaults:

| Alert | Threshold | Duration |
|---|---:|---:|
| Disk | 85% on any monitored filesystem | 5 minutes |
| Memory | 90% | 5 minutes |
| CPU | 95% | 10 minutes |
| Offline | Disconnected | 2 minutes |

Override the module's `alerts` map to change fleet policy. Values are
`percentage:minutes`, with `Status = "0:2"` for a two-minute offline alert.
Notification delivery is configured on the Beszel hub separately.

Agent enrollment credentials are written to `/etc/beszel/agent.env` with mode
0600. The agent makes outbound HTTPS/WebSocket connections; it does not open an
SSH listener. The pinned binary is verified against the upstream release checksum.
The service restarts on failure and starts at boot.

Terraform detects changes to Beszel registrations and alert rules through the
provider. Changes to host inputs or the monitoring role rerun Ansible. Host file
changes made outside Terraform are not polled continuously; explicitly replace
`module.host.terraform_data.configuration` to rerun Ansible when repairing drift.
Host deletion is protected with `prevent_destroy`; retirement needs deliberate
review because deleting a Beszel system deletes monitoring history.

## Develop

```sh
node --test infra/terraform/tests/filesystem-discovery.test.ts
cd infra/ansible
ansible-playbook -i inventory/prod.example.yml playbooks/monitoring.yml --syntax-check
```

Never run the full host playbook as a smoke test on an existing server. Use the
monitoring-only playbook when testing agent changes without changing SSH,
firewall, Docker, or storage configuration.
