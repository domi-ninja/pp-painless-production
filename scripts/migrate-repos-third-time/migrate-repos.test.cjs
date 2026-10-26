const { test } = require('node:test');
const assert = require('node:assert/strict');
const { getForgejoToken } = require('./migrate-repos.cjs');

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
