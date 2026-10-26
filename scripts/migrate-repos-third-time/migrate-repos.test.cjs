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


function migrationFixture(destinationExists) {
  const fs = require('node:fs');
  const vm = require('node:vm');
  const commands = [];
  let exists = destinationExists;
  const childProcess = {
    execFileSync(name, args) {
      assert.equal(name, 'git');
      const command = args.slice(args.indexOf('-C') + 2);
      if (command.join(' ') === 'remote get-url upstream') return 'ssh://git@git.domi.ninja:2222/domi-ninja/app.git';
      if (command[0] === 'rev-parse') return 'true';
      if (command[0] === 'for-each-ref') return command[1].includes('objectname') ? 'refs/heads/main:abc' : 'main';
      throw new Error(`Unexpected git read: ${command}`);
    },
    spawnSync(name, args) {
      assert.equal(name, 'git');
      commands.push(args.slice(args.indexOf('-C') + 2));
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

test('keeps existing Forgejo destinations on the skip path', async () => {
  const {api, commands} = migrationFixture(true);
  assert.equal(await api.migrateRepo('/root/repos/app'), 'skipped');
  assert.ok(!commands.some(command => command[0] === 'push'));
});
