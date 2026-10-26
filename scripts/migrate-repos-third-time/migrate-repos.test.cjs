const { test } = require('node:test');
const assert = require('node:assert/strict');
const { getForgejoToken, verifyDestination, validatePlan, main } = require('./migrate-repos.cjs');

test('requires an operator-managed token without creating one remotely', () => {
  const previous = process.env.FORGEJO_TOKEN;
  delete process.env.FORGEJO_TOKEN;
  try {
    assert.throws(getForgejoToken, /operator-managed migration token/);
    process.env.FORGEJO_TOKEN = 'test-token';
    assert.equal(getForgejoToken(), 'test-token');
  } finally {
    if (previous === undefined) delete process.env.FORGEJO_TOKEN;
    else process.env.FORGEJO_TOKEN = previous;
  }
});


test('requires a reviewed plan before migrating', async () => {
  await assert.rejects(main([]), /Usage:/);
  await assert.rejects(main(['--apply']), /Usage:/);
  assert.throws(() => validatePlan([]), /must contain/);
  assert.throws(() => validatePlan([{repoPath: '/tmp/not-an-approved-repo'}]), /Not a repository inside/);
});

test('rejects a public or mismatched destination', () => {
  verifyDestination({ full_name: 'domi-ninja/app', private: true }, 'app');
  assert.throws(() => verifyDestination({full_name:'another-owner/app', private:true}, 'app'), /unverified/);
  assert.throws(() => verifyDestination({full_name:'domi-ninja/app', private:false}, 'app'), /public/);
});


function migrationFixture(destinationExists, failPush = false, upstreamUrl = 'ssh://git@git.domi.ninja:2222/domi-ninja/app.git') {
  const fs = require('node:fs');
  const vm = require('node:vm');
  const commands = [];
  let exists = destinationExists;
  const childProcess = {
    execFileSync(name, args) {
      assert.equal(name, 'git');
      const command = args.slice(args.indexOf('-C') + 2);
      if (command.join(' ') === 'remote get-url upstream') return upstreamUrl;
      if (command[0] === 'rev-parse') return 'true';
      if (command[0] === 'for-each-ref') return command[1].includes('objectname') ? 'refs/heads/main:abc' : 'main';
      throw new Error(`Unexpected git read: ${command}`);
    },
    spawnSync(name, args) {
      assert.equal(name, 'git');
      const command = args.slice(args.indexOf('-C') + 2);
      commands.push(command);
      if (failPush && command[0] === 'push') return {status: 1, stdout: '', stderr: 'push rejected'};
      return {status: 0, stdout: '', stderr: ''};
    },
  };
  const context = {
    require(name) { return name === 'node:child_process' ? childProcess : require(name); },
    module: {exports: {}}, process: {env: {FORGEJO_TOKEN: 'fixture-token'}}, URL,
    console: {log() {}, error() {}},
    async fetch(url, {method}) {
      let status = 200;
      let body = {full_name: 'domi-ninja/app', private: true};
      if (method === 'POST') exists = true;
      else if (url.includes('/orgs/') || !exists) {status = 404; body = {message: 'missing'};}
      return {ok: status === 200, status, text: async () => JSON.stringify(body)};
    },
  };
  vm.runInNewContext(fs.readFileSync(require.resolve('./migrate-repos.cjs'), 'utf8'), context);
  return {api: context.module.exports, commands};
}

test('repopulates a recreated destination before setting branch upstreams', async () => {
  const {api, commands} = migrationFixture(false);
  assert.equal(await api.migrateRepo('/root/repos/app'), 'migrated');
  const push = commands.findIndex(command => command.join(' ') === 'push upstream --all');
  const fetch = commands.findIndex(command => command[0] === 'fetch');
  assert.ok(push >= 0 && fetch > push);
});

test('pushes before setting upstreams when retrying an existing destination', async () => {
  const {api, commands} = migrationFixture(true);
  assert.equal(await api.migrateRepo('/root/repos/app'), 'migrated');
  const push = commands.findIndex(command => command.join(' ') === 'push upstream --all');
  const fetch = commands.findIndex(command => command[0] === 'fetch');
  assert.ok(push >= 0 && fetch > push);
});

test('does not set branch upstreams after a rejected non-forced push', async () => {
  const {api, commands} = migrationFixture(true, true);
  await assert.rejects(api.migrateRepo('/root/repos/app'), /push rejected/);
  assert.ok(!commands.some(command => command[0] === 'fetch' || command[0] === 'branch'));
});


test('allows dot-prefixed repository names without allowing parent traversal', () => {
  const {api} = migrationFixture(true);
  assert.equal(api.planRepo('/root/repos/..legacy').repoPath, '/root/repos/..legacy');
  assert.throws(() => api.planRepo('/root/repos/../escape'), /Not a repository inside/);
  assert.throws(() => api.planRepo('/root/repos/..'), /Not a repository inside/);
});


test('rejects destinations differing only by case before any mutation', () => {
  const {api, commands} = migrationFixture(true, false, '');
  const entries = [api.planRepo('/root/repos/App'), api.planRepo('/root/repos/app')];
  assert.throws(() => api.validatePlan(entries), /Duplicate destination/);
  assert.deepEqual(commands, []);
});
