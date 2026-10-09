const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');

function readEnv(file) {
  const values = Object.create(null);
  if (!fs.existsSync(file)) return values;
  if (fs.lstatSync(file).isSymbolicLink()) throw new Error('Env file must not be a symlink');
  fs.readFileSync(file, 'utf8').split(/\r?\n/).forEach((raw, index) => {
    const line = raw.trim();
    if (!line || line.startsWith('#')) return;
    const separator = line.indexOf('=');
    const key = line.slice(0, separator).trim();
    let value = line.slice(separator + 1).trim();
    if (separator < 0 || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || Object.hasOwn(values, key)) {
      throw new Error(`Invalid or repeated env key on line ${index + 1}`);
    }
    if (value.startsWith("'") || value.startsWith('"')) {
      if (value.length < 2 || value.at(-1) !== value[0]) throw new Error(`Unmatched quote on line ${index + 1}`);
      value = value.slice(1, -1);
    }
    values[key] = value;
  });
  return values;
}

function writeEnv(file, values) {
  if (fs.existsSync(file) && fs.lstatSync(file).isSymbolicLink()) throw new Error('Env file must not be a symlink');
  const directory = path.dirname(file);
  fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
  const temp = path.join(directory, `.pp-env-${crypto.randomBytes(12).toString('hex')}`);
  const body = Object.keys(values).sort().map(key => {
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || /[\r\n]/.test(values[key])) throw new Error('Env values must be single-line');
    return `${key}='${values[key]}'\n`;
  }).join('');
  try {
    fs.writeFileSync(temp, body, { mode: 0o600, flag: 'wx' });
    fs.renameSync(temp, file);
  } finally {
    if (fs.existsSync(temp)) fs.unlinkSync(temp);
  }
}

module.exports = { readEnv, writeEnv };
if (require.main === module) {
  try {
    const [action, file, key] = process.argv.slice(2);
    const values = readEnv(file);
    if (action === 'get') process.stdout.write(values[key] ?? '');
    else if (action === 'set') {
      values[key] = fs.readFileSync(0, 'utf8');
      writeEnv(file, values);
    } else throw new Error('Use get or set');
  } catch (error) { console.error(error.message); process.exitCode = 1; }
}
