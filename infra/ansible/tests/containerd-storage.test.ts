import { test } from "node:test";
import assert from "node:assert/strict";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  existsSync,
  rmSync,
  symlinkSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";

const original = readFileSync(
  new URL(
    "../roles/containerd_data_root/files/relocate-containerd.sh",
    import.meta.url,
  ),
  "utf8",
);

// Redirect filesystem paths into a fixture and replace daemon commands. rsync,
// file operations, rollback, and health checks execute the actual helper logic.
function fixture() {
  const root = mkdtempSync(join(tmpdir(), "pp-containerd-test-"));
  for (const dir of [
    "data",
    "var/lib/containerd",
    "etc/containerd",
    "etc/systemd/system",
    "run",
    "bin",
  ])
    mkdirSync(join(root, dir), { recursive: true });
  const config = join(root, "etc/containerd/config.toml");
  const source = join(root, "var/lib/containerd");
  const target = join(root, "data/containerd");
  writeFileSync(config, `root = '${source}'\nversion = 2\n`);
  writeFileSync(join(source, "snapshot"), "persistent image data");
  const helper = join(root, "relocate.sh");
  writeFileSync(
    helper,
    original
      .replaceAll("/data", `${root}/data`)
      .replaceAll("/var/lib/containerd", source)
      .replaceAll("/etc/", `${root}/etc/`)
      .replaceAll("/run/", `${root}/run/`)
      .replace("[[ $EUID == 0 ]]", "true"),
  );
  const command = (name: string, body: string) =>
    writeFileSync(
      join(root, "bin", name),
      `#!/usr/bin/env bash\nset -eu\n${body}\n`,
      { mode: 0o755 },
    );
  command("mountpoint", "[[ ${NO_MOUNT:-0} == 0 ]]");
  command("getent", "exit 0");
  command("findmnt", "exit 0");
  command("containerd", 'cat "$FIXTURE/etc/containerd/config.toml"');
  command(
    "systemctl",
    `echo "$*" >>"$FIXTURE/commands"
if [[ $FAIL_START == 1 && $1 == start && ! -f $FIXTURE/failed && $(head -1 "$FIXTURE/etc/containerd/config.toml") == *data/containerd* ]]; then touch "$FIXTURE/failed"; exit 1; fi`,
  );
  command(
    "docker",
    `echo "$*" >>"$FIXTURE/commands"
case $1 in
 ps) if [[ $2 == -q ]]; then echo test-container; else echo 'test-container Up healthy'; fi;;
 inspect) echo 'true healthy';;
 start|stop) :;;
 *) exit 1;;
esac`,
  );
  const run = (extra: Record<string, string> = {}) =>
    spawnSync("bash", [helper, target], {
      encoding: "utf8",
      timeout: 15000,
      env: {
        ...process.env,
        FIXTURE: root,
        FAIL_START: "0",
        PATH: `${root}/bin:${process.env.PATH}`,
        ...extra,
      },
    });
  return {
    root,
    source,
    target,
    config,
    run,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  };
}

test("copies the store, restores containers, removes the old copy, and is idempotent", () => {
  const f = fixture();
  try {
    const first = f.run();
    assert.equal(first.status, 0, first.stdout + first.stderr);
    assert.equal(
      readFileSync(join(f.target, "snapshot"), "utf8"),
      "persistent image data",
    );
    assert.equal(existsSync(join(f.source, "snapshot")), false);
    assert.match(readFileSync(f.config, "utf8"), /data\/containerd/);
    assert.match(
      readFileSync(join(f.root, "commands"), "utf8"),
      /start test-container/,
    );
    const second = f.run();
    assert.equal(second.status, 0, second.stderr);
    assert.match(second.stdout, /unchanged/);
  } finally {
    f.cleanup();
  }
});

test("a daemon startup failure restores the original store and configuration", () => {
  const f = fixture();
  try {
    const result = f.run({ FAIL_START: "1" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /restoring original configuration/);
    assert.equal(
      readFileSync(join(f.source, "snapshot"), "utf8"),
      "persistent image data",
    );
    assert.equal(
      readFileSync(f.config, "utf8"),
      `root = '${f.source}'\nversion = 2\n`,
    );
    assert.match(
      readFileSync(join(f.root, "commands"), "utf8"),
      /start test-container/,
    );
  } finally {
    f.cleanup();
  }
});

test("refuses an absent data mount, existing destination, and destination symlink before stopping services", () => {
  for (const scenario of ["unmounted", "occupied", "symlink"]) {
    const f = fixture();
    try {
      if (scenario === "occupied") mkdirSync(f.target);
      if (scenario === "symlink") symlinkSync(f.source, f.target);
      const result = f.run(scenario === "unmounted" ? { NO_MOUNT: "1" } : {});
      assert.notEqual(result.status, 0, scenario);
      assert.equal(existsSync(join(f.root, "commands")), false, scenario);
      assert.equal(
        readFileSync(join(f.source, "snapshot"), "utf8"),
        "persistent image data",
      );
    } finally {
      f.cleanup();
    }
  }
});
