const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { readEnv, writeEnv } = require('./env-file.cjs');

function bootstrap(args) {
  const root = path.resolve(__dirname, '..');
  let file = path.join(root, '.env.prod');
  let project = process.env.PP_PROJECT;
  let hostname;
  let rotate = false;
  let restore = false;
  for (let i = 0; i < args.length; i++) {
    switch (args[i]) {
      case '--env-file':
        if (!args[i + 1] || args[i + 1].startsWith('--')) throw new Error('Supply an env-file path');
        file = path.resolve(args[++i]); break;
      case '--project': project = args[++i]; break;
      case '--rotate': rotate = true; break;
      case '--restore': restore = true; break;
      default:
        if (args[i].startsWith('-') || hostname) throw new Error('Unsupported bootstrap argument');
        hostname = args[i];
    }
  }
  const backup = `${file}.previous`;
  if (restore) {
    if (rotate) throw new Error('Choose restore or rotate');
    if (!fs.existsSync(backup)) throw new Error('No previous environment copy');
    writeEnv(file, readEnv(backup));
    console.log('Restored env values. The recovery copy remains. Deploy the saved application inputs to apply them.');
    return;
  }
  if (!hostname || !/^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$/.test(hostname)) throw new Error('Supply a site hostname');
  if (!project || !/^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(project)) throw new Error('Supply --project with the deployment project slug');
  if (rotate && fs.existsSync(backup)) throw new Error('Archive or restore the outstanding recovery copy before another rotation');
  if (rotate && !fs.existsSync(file)) throw new Error('Initialize the environment before rotating');
  const values = readEnv(file);
  const original = { ...values };
  const usable = value => value && !/[<>]/.test(value);
  const setDefault = (key, fallback) => { if (!usable(values[key])) values[key] = fallback; };
  const secret = (key, bytes) => { if (rotate || !usable(values[key])) values[key] = crypto.randomBytes(bytes).toString('hex'); };
  setDefault('VITE_CONVEX_URL', `https://api.${hostname}`);
  setDefault('SITE_URL', `https://${hostname}`);
  setDefault('CONVEX_SELF_HOSTED_URL', `https://api.${hostname}`);
  setDefault('INSTANCE_NAME', project);
  setDefault('POSTGRES_DB', project.replaceAll('-', '_'));
  setDefault('POSTGRES_USER', project.replaceAll('-', '_'));
  setDefault('STORAGE_ACCESS_KEY', crypto.randomBytes(12).toString('hex'));
  for (const key of ['INSTANCE_SECRET', 'POSTGRES_PASSWORD', 'STORAGE_SECRET_KEY']) secret(key, 32);
  for (const [key, suffix] of Object.entries({ EXPORTS: 'exports', SNAPSHOT_IMPORTS: 'snapshot-imports', MODULES: 'modules', FILES: 'files', SEARCH: 'search' })) {
    setDefault(`S3_STORAGE_${key}_BUCKET`, `${project}-convex-${suffix}`);
  }
  if (rotate || !usable(values.POSTGRES_URL)) {
    values.POSTGRES_URL = `postgres://${encodeURIComponent(values.POSTGRES_USER)}:${encodeURIComponent(values.POSTGRES_PASSWORD)}@postgres:5432/${encodeURIComponent(values.POSTGRES_DB)}?sslmode=disable`;
  }
  const privateExists = usable(values.JWT_PRIVATE_KEY);
  const publicExists = usable(values.JWKS);
  if (!rotate && Boolean(privateExists) !== Boolean(publicExists)) throw new Error('Keep both JWT key values or explicitly rotate the pair');
  if (rotate || !privateExists) {
    const keys = crypto.generateKeyPairSync('rsa', { modulusLength: 2048 });
    values.JWT_PRIVATE_KEY = keys.privateKey.export({ type: 'pkcs8', format: 'pem' }).trimEnd().replaceAll('\n', ' ');
    values.JWKS = JSON.stringify({ keys: [{ use: 'sig', ...keys.publicKey.export({ format: 'jwk' }) }] });
  }
  if (rotate) {
    delete values.CONVEX_SELF_HOSTED_ADMIN_KEY;
    fs.chmodSync(file, 0o600);
    fs.copyFileSync(file, backup, fs.constants.COPYFILE_EXCL);
    fs.chmodSync(backup, 0o600);
  }
  if (JSON.stringify(original) !== JSON.stringify(values)) writeEnv(file, values);
  else if (fs.existsSync(file)) fs.chmodSync(file, 0o600);
  console.log(rotate ? 'Rotated env credentials. Deploy to apply them; keep the recovery copy until verified.' : 'Initialized env values. Existing credentials and application keys were preserved.');
}
module.exports = { bootstrap };
if (require.main === module) {
  try { bootstrap(process.argv.slice(2)); }
  catch (error) { console.error(error.message); process.exitCode = 1; }
}
