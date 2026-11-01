import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

type Config = {
  name: string;
  address: string;
  user: string;
  local: boolean;
  configure_base: boolean;
  variables: Record<string, unknown>;
};

const encoded = process.env.PP_HOST_CONFIG;
if (!encoded) throw new Error('Terraform must provide PP_HOST_CONFIG');
const config: Config = JSON.parse(encoded);
delete process.env.PP_HOST_CONFIG;
const ansible = fileURLToPath(new URL('../../../ansible/', import.meta.url));
const temporary = mkdtempSync(join(tmpdir(), 'pp-host-'));
try {
  const inventory = {
    prod_servers: {
      hosts: {
        [config.name]: {
          ansible_host: config.address,
          ansible_user: config.user,
          ansible_connection: config.local ? 'local' : 'ssh',
        },
      },
    },
  };
  writeFileSync(join(temporary, 'inventory.json'), JSON.stringify(inventory), { mode: 0o600 });
  writeFileSync(join(temporary, 'vars.json'), JSON.stringify(config.variables), { mode: 0o600 });
  const playbook = config.configure_base ? 'prod-server.yml' : 'monitoring.yml';
  const result = spawnSync('ansible-playbook', [
    '-i', join(temporary, 'inventory.json'),
    '-e', '@' + join(temporary, 'vars.json'),
    'playbooks/' + playbook,
  ], {
    cwd: ansible,
    stdio: 'inherit',
    env: { ...process.env, ANSIBLE_CONFIG: join(ansible, 'ansible.cfg') },
    timeout: 1_800_000,
  });
  if (result.error) throw new Error('Host provisioning process failed');
  if (result.status !== 0) throw new Error(`Ansible exited with status ${result.status}`);
} finally {
  rmSync(temporary, { recursive: true, force: true });
}
