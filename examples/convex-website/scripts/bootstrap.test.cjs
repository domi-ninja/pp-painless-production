const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { bootstrap } = require('./bootstrap-env.cjs');
const { readEnv, writeEnv } = require('./env-file.cjs');
const { storagePolicy } = require('./storage-policy.cjs');

test('initialization preserves app values and explicit rotation keeps a recovery copy', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pp-convex-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, '.env.prod');
  const args = ['site.example.com', '--project', 'shop', '--env-file', file];
  writeEnv(file, { APP_SECRET: "literal $value # 'quoted'", __proto__: 'ignored' });
  bootstrap(args);
  const before = readEnv(file);
  bootstrap(args);
  assert.deepEqual(readEnv(file), before);
  assert.equal(fs.statSync(file).mode & 0o777, 0o600);
  bootstrap([...args, '--rotate']);
  const after = readEnv(file);
  assert.notEqual(after.INSTANCE_SECRET, before.INSTANCE_SECRET);
  assert.notEqual(after.JWT_PRIVATE_KEY, before.JWT_PRIVATE_KEY);
  for (const key of ['APP_SECRET', 'INSTANCE_NAME', 'POSTGRES_DB', 'STORAGE_ACCESS_KEY', 'S3_STORAGE_FILES_BUCKET']) assert.equal(after[key], before[key]);
  assert.deepEqual(readEnv(`${file}.previous`), before);
  assert.throws(() => bootstrap([...args, '--rotate']), /outstanding/);
  bootstrap(['--env-file', file, '--restore']);
  assert.deepEqual(readEnv(file), before);
  assert.ok(fs.existsSync(`${file}.previous`));
});
test('env parsing rejects symlinks and preserves literal values', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pp-env-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const file = path.join(root, 'values');
  writeEnv(file, { ['__proto__']: 'literal', APP: "a'b\"c $literal # text" });
  assert.equal(readEnv(file).__proto__, 'literal');
  assert.equal(readEnv(file).APP, "a'b\"c $literal # text");
  fs.symlinkSync(file, path.join(root, 'link'));
  assert.throws(() => readEnv(path.join(root, 'link')), /symlink/);
  assert.throws(() => bootstrap(['--env-file']), /path/);
});
test('storage policy restricts resources and excludes admin actions', () => {
  const policy = storagePolicy(['shop-files', 'shop-modules']);
  assert.deepEqual(policy.Statement[0].Resource, ['arn:aws:s3:::shop-files', 'arn:aws:s3:::shop-modules']);
  assert.ok(policy.Statement.flatMap(s => s.Action).every(action => !action.includes('*') && !action.includes('Policy') && !action.includes('DeleteBucket')));
  assert.throws(() => storagePolicy(['*']), /bucket/);
});
