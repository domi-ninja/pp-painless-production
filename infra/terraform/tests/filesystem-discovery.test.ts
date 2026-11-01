import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';

test('discovers data disks once and omits root aliases and container mount types', () => {
  const temp=mkdtempSync(join(tmpdir(),'beszel-filesystems-'));
  try {
    mkdirSync(join(temp,'bin'));
    writeFileSync(join(temp,'bin/findmnt'), '#!/bin/bash\n[[ "$*" == *"ext2,ext3,ext4,xfs,btrfs,zfs"* ]] || exit 1\nprintf "8:1 /\\n8:1 /root-alias\\n8:2 /data\\n8:2 /data-alias\\n8:3 /mnt/volume\\n"\n', {mode:0o755});
    const output=join(temp,'filesystems.env');
    const source=readFileSync(new URL('../../ansible/roles/beszel_agent/files/discover-filesystems.sh',import.meta.url),'utf8').replace('/run/beszel-agent/filesystems.env',output);
    execFileSync('bash',['-c',source],{env:{...process.env,PATH:join(temp,'bin')+':'+process.env.PATH}});
    assert.equal(readFileSync(output,'utf8'),'EXTRA_FILESYSTEMS=/data,/mnt/volume\n');
  }finally{rmSync(temp,{recursive:true,force:true});}
});
