#!/usr/bin/env node
'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { execFileSync, spawnSync } = require('node:child_process');

const REPOS_DIR = '/root/repos';
const REMOTE_NAME = 'upstream';
const FORGEJO_HOST = 'git.domi.ninja';
const FORGEJO_OWNER = 'domi-ninja';
const FORGEJO_SSH_PORT = '2222';
const FORGEJO_API_BASE = `https://${FORGEJO_HOST}/api/v1`;
const rawGitDirs = new Set();
let forgejoToken = '';

function runGit(repoPath, args) {
  const gitLocationArgs = rawGitDirs.has(repoPath)
    ? ['--git-dir', repoPath]
    : ['-c', 'safe.directory=*', '-C', repoPath];

  return execFileSync('git', [...gitLocationArgs, ...args], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

function tryGit(repoPath, args) {
  try {
    return runGit(repoPath, args);
  } catch {
    return null;
  }
}

function spawnGit(repoPath, args) {
  const gitLocationArgs = rawGitDirs.has(repoPath)
    ? ['--git-dir', repoPath]
    : ['-c', 'safe.directory=*', '-C', repoPath];

  const result = spawnSync('git', [...gitLocationArgs, ...args], {
    encoding: 'utf8',
    stdio: 'pipe',
  });

  if (result.status !== 0) {
    const stderr = result.stderr ? `\n${result.stderr.trim()}` : '';
    const stdout = result.stdout ? `\n${result.stdout.trim()}` : '';
    throw new Error(`git ${args.join(' ')} failed in ${repoPath}${stderr}${stdout}`);
  }
}

function getForgejoToken() {
  if (forgejoToken) return forgejoToken;
  forgejoToken = (process.env.FORGEJO_TOKEN || '').trim();
  if (!forgejoToken) {
    throw new Error('Set FORGEJO_TOKEN to an operator-managed migration token; this script does not create access tokens.');
  }
  return forgejoToken;
}

function isGitRepo(repoPath) {
  if (
    tryGit(repoPath, ['rev-parse', '--is-inside-work-tree']) === 'true' ||
    tryGit(repoPath, ['rev-parse', '--is-bare-repository']) === 'true'
  ) {
    return true;
  }

  if (
    fs.existsSync(path.join(repoPath, 'HEAD')) &&
    fs.existsSync(path.join(repoPath, 'config')) &&
    fs.existsSync(path.join(repoPath, 'objects')) &&
    fs.existsSync(path.join(repoPath, 'refs'))
  ) {
    rawGitDirs.add(repoPath);
    return true;
  }

  return false;
}

function listRepoDirs() {
  return fs
    .readdirSync(REPOS_DIR, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => path.join(REPOS_DIR, entry.name))
    .filter(isGitRepo)
    .sort();
}

function parseGitRemoteUrl(remoteUrl) {
  const scpLike = remoteUrl.match(/^([^@:\s]+)@([^:\s]+):(.+)$/);
  if (scpLike) {
    return {
      user: scpLike[1],
      host: scpLike[2],
      port: '',
      pathname: `/${scpLike[3]}`,
      sshLike: true,
    };
  }

  try {
    const parsed = new URL(remoteUrl);
    return {
      user: decodeURIComponent(parsed.username || ''),
      host: parsed.hostname,
      port: parsed.port,
      pathname: parsed.pathname,
      sshLike: parsed.protocol === 'ssh:',
    };
  } catch {
    return {
      user: '',
      host: '',
      port: '',
      pathname: remoteUrl,
      sshLike: false,
    };
  }
}

function isForgejoSshUpstream(remoteUrl) {
  const parsed = parseGitRemoteUrl(remoteUrl);
  if (parsed.host !== FORGEJO_HOST) return false;
  if (!parsed.sshLike) return false;
  if (parsed.user && parsed.user !== 'git') return false;
  if (parsed.port && parsed.port !== FORGEJO_SSH_PORT) return false;
  return true;
}

function repoNameFromPathname(pathname) {
  const lastSegment = pathname.split('/').filter(Boolean).pop() || '';
  return lastSegment.replace(/\.git$/, '');
}

function sanitizeRepoName(name) {
  return name
    .trim()
    .replace(/\.git$/, '')
    .replace(/[^A-Za-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function deriveRepoName(repoPath, upstreamUrl) {
  if (upstreamUrl) {
    const fromRemote = sanitizeRepoName(repoNameFromPathname(parseGitRemoteUrl(upstreamUrl).pathname));
    if (fromRemote) return fromRemote;
  }

  return sanitizeRepoName(path.basename(repoPath));
}

function remoteNameFromDomain(remoteUrl) {
  const parsed = parseGitRemoteUrl(remoteUrl);
  const host = parsed.host || 'previous-upstream';
  const cleaned = host.replace(/^www\./, '').replace(/^(git|ssh)\./, '');
  const parts = cleaned.split('.');
  const name = parts.length > 1 ? parts[0] : cleaned;
  return name.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'previous-upstream';
}

function uniqueRemoteName(repoPath, wanted) {
  const remotes = new Set((tryGit(repoPath, ['remote']) || '').split(/\s+/).filter(Boolean));
  if (!remotes.has(wanted)) return wanted;

  for (let index = 2; index < 100; index += 1) {
    const candidate = `${wanted}-${index}`;
    if (!remotes.has(candidate)) return candidate;
  }

  throw new Error(`Could not find available remote name for ${wanted}`);
}

function forgejoSshUrl(repoName) {
  return `ssh://git@${FORGEJO_HOST}:${FORGEJO_SSH_PORT}/${FORGEJO_OWNER}/${repoName}.git`;
}

function listLocalBranches(repoPath) {
  return runGit(repoPath, ['for-each-ref', '--format=%(refname:short)', 'refs/heads'])
    .split('\n')
    .map((branch) => branch.trim())
    .filter(Boolean);
}

function setForgejoBranchUpstreams(repoPath) {
  const branches = listLocalBranches(repoPath);
  if (branches.length === 0) {
    console.log('    no local branches found');
    return;
  }

  console.log(`    fetch ${REMOTE_NAME}`);
  spawnGit(repoPath, ['fetch', REMOTE_NAME, '--prune']);

  for (const branch of branches) {
    console.log(`    set ${branch} -> ${REMOTE_NAME}/${branch}`);
    spawnGit(repoPath, ['branch', `--set-upstream-to=${REMOTE_NAME}/${branch}`, branch]);
  }
}

async function requestJson(method, endpoint, body) {
  const url = `${FORGEJO_API_BASE}${endpoint}`;
  const response = await fetch(url, {
    method,
    headers: {
      Accept: 'application/json',
      Authorization: `token ${getForgejoToken()}`,
      ...(body ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });

  const text = await response.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!response.ok) {
    const message = data && data.message ? data.message : text;
    const error = new Error(`${method} ${url} returned ${response.status}: ${message}`);
    error.status = response.status;
    throw error;
  }

  return data;
}

async function requestStatus(method, endpoint, body) {
  try {
    const data = await requestJson(method, endpoint, body);
    return { status: 200, data };
  } catch (error) {
    return { status: error.status || 0, error };
  }
}

async function forgejoRepoExists(repoName) {
  const endpoint = `/repos/${encodeURIComponent(FORGEJO_OWNER)}/${encodeURIComponent(repoName)}`;
  const response = await requestStatus('GET', endpoint);
  if (response.status === 200) return true;
  if (response.status === 404) return false;
  throw response.error;
}

async function forgejoOwnerIsOrg() {
  const response = await requestStatus('GET', `/orgs/${encodeURIComponent(FORGEJO_OWNER)}`);
  if (response.status === 200) return true;
  if (response.status === 404) return false;
  throw response.error;
}

async function createForgejoRepo(repoName) {
  const body = {
    name: repoName,
    private: true,
    auto_init: false,
  };

  const endpoint = (await forgejoOwnerIsOrg())
    ? `/org/${encodeURIComponent(FORGEJO_OWNER)}/repos`
    : '/user/repos';
  await requestJson('POST', endpoint, body);
}

async function ensureForgejoRepo(repoName) {
  if (await forgejoRepoExists(repoName)) {
    console.log(`    Forgejo repo exists: ${FORGEJO_OWNER}/${repoName}`);
    return;
  }

  console.log(`    creating private Forgejo repo: ${FORGEJO_OWNER}/${repoName}`);
  await createForgejoRepo(repoName);
}

async function migrateRepo(repoPath) {
  const upstreamUrl = tryGit(repoPath, ['remote', 'get-url', REMOTE_NAME]);
  if (upstreamUrl && isForgejoSshUpstream(upstreamUrl)) {
    console.log(`  skip: ${REMOTE_NAME} already points to Forgejo SSH (${upstreamUrl})`);
    setForgejoBranchUpstreams(repoPath);
    return 'skipped';
  }

  const repoName = deriveRepoName(repoPath, upstreamUrl);
  if (!repoName) {
    throw new Error(`Could not derive repo name for ${repoPath}`);
  }

  console.log(`  migrate: ${repoName}`);
  await ensureForgejoRepo(repoName);

  if (upstreamUrl) {
    const oldRemoteName = uniqueRemoteName(repoPath, remoteNameFromDomain(upstreamUrl));
    console.log(`    rename ${REMOTE_NAME} -> ${oldRemoteName}`);
    spawnGit(repoPath, ['remote', 'rename', REMOTE_NAME, oldRemoteName]);
  } else {
    console.log(`    no ${REMOTE_NAME} remote found`);
  }

  const nextUpstreamUrl = forgejoSshUrl(repoName);
  console.log(`    add ${REMOTE_NAME} ${nextUpstreamUrl}`);
  spawnGit(repoPath, ['remote', 'add', REMOTE_NAME, nextUpstreamUrl]);

  console.log(`    push all branches to ${REMOTE_NAME}`);
  spawnGit(repoPath, ['push', REMOTE_NAME, '--all']);
  setForgejoBranchUpstreams(repoPath);

  return 'migrated';
}

async function main() {
  if (!fs.existsSync(REPOS_DIR)) {
    throw new Error(`Repos directory does not exist: ${REPOS_DIR}`);
  }

  console.log(`Repos: ${REPOS_DIR}`);
  console.log(`Forgejo: ${FORGEJO_HOST} owner=${FORGEJO_OWNER} ssh_port=${FORGEJO_SSH_PORT}`);
  console.log('');

  let migrated = 0;
  let skipped = 0;
  let failed = 0;

  for (const repoPath of listRepoDirs()) {
    console.log(path.relative(REPOS_DIR, repoPath));

    try {
      const result = await migrateRepo(repoPath);
      if (result === 'skipped') skipped += 1;
      else migrated += 1;
    } catch (error) {
      failed += 1;
      console.error(`  failed: ${error.message}`);
    }

    console.log('');
  }

  console.log(`Done. migrated=${migrated} skipped=${skipped} failed=${failed}`);
  if (failed > 0) process.exitCode = 1;
}

if (require.main === module) {
  main().catch((error) => {
    console.error(error.message);
    process.exitCode = 1;
  });
}

module.exports = { getForgejoToken };

