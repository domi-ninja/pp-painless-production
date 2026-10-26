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
