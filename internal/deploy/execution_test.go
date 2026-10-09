package deploy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrationTimeoutRemovesOnlyItsContainerAndReportsCleanupFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "cleanup", true: "cleanup failure"}[fail], func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(root, "container")
			t.Setenv("PP_TEST_CONTAINER", marker)
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			if fail {
				t.Setenv("PP_TEST_CLEANUP_FAIL", "1")
			}
			writeExecutable(t, root, "docker", `#!/bin/sh
case "$1" in
create) printf '%s' "$3" > "$PP_TEST_CONTAINER";;
start) exec sleep 30;;
ps) [ ! -f "$PP_TEST_CONTAINER" ] || cat "$PP_TEST_CONTAINER";;
inspect) cat "$PP_TEST_CONTAINER";;
rm) [ "${PP_TEST_CLEANUP_FAIL:-}" != 1 ] || exit 2
    [ "$3" = "$(cat "$PP_TEST_CONTAINER")" ] || exit 3
    rm "$PP_TEST_CONTAINER";;
*) exit 4;;
esac
`)
			var out bytes.Buffer
			d := NewDeployer(root, &out, &out)
			err := d.runMigration(&Migration{Command: []string{"sleep", "30"}, TimeoutSeconds: 1}, "fixture", false)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("missing timeout: %v", err)
			}
			_, statErr := os.Stat(marker)
			if fail {
				if !strings.Contains(err.Error(), "remove migration container pp-migration-") || statErr != nil {
					t.Fatalf("lost cleanup failure: %v %v", err, statErr)
				}
			} else if !os.IsNotExist(statErr) {
				t.Fatalf("container remains: %v", statErr)
			}
		})
	}
}

func TestImageManifestMergesOwnershipWithoutMergingDifferentIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "images.json")
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("b", 64)
	input := imageManifest{Version: 1, Images: []ManagedImage{
		{Reference: "image:tag", ID: first, Source: true},
		{Reference: "image:tag", ID: first, Built: true},
		{Reference: "image:tag", ID: second},
	}}
	if err := writeImageManifest(path, input); err != nil {
		t.Fatal(err)
	}
	got, err := readImageManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 2 || !got.Images[0].Built || !got.Images[0].Source || got.Images[1].ID != second {
		t.Fatalf("lost image ownership: %+v", got)
	}
}

func TestRecoveryRejectsMissingRoutesAndImageLocksBeforeDatabaseChanges(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	current, err := LoadRelease(root, project, "current")
	if err != nil {
		t.Fatal(err)
	}
	target, err := LoadRelease(root, project, "previous")
	if err != nil {
		t.Fatal(err)
	}
	contract := recoveryContract{Version: 1, RouteHosts: []string{"host"}}
	if err := writeJSON(filepath.Join(target.BundlePath, "recovery.json"), contract); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "database-command")
	t.Setenv("PP_TEST_DB_COMMAND", marker)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, root, "docker", "#!/bin/sh\ntouch \"$PP_TEST_DB_COMMAND\"\n")
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	if _, _, err := d.recoverRelease(current, &target, true); err == nil || !strings.Contains(err.Error(), "recovery route") {
		t.Fatalf("missing route accepted: %v", err)
	}
	writeNestedFile(t, root, mustRelative(t, root, filepath.Join(target.BundlePath, "hosts", "host", "routes.caddy")), "# saved\n")
	writeNestedFile(t, root, mustRelative(t, root, filepath.Join(target.BundlePath, "hosts", "host", "compose.yml")), "services:\n  web:\n    image: pp.local/shop/prod/web:previous\n")
	if _, _, err := d.recoverRelease(current, &target, true); err == nil || !strings.Contains(err.Error(), "image lock") {
		t.Fatalf("missing lock accepted: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("database changed before validation: %v", err)
	}
}

func TestPhasedApplyRunsInfrastructureHookBeforeBackend(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "commands")
	t.Setenv("PP_TEST_COMMAND_LOG", log)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, root, "ssh", `#!/bin/sh
printf '%s\n' "$*" >> "$PP_TEST_COMMAND_LOG"
case "$*" in *"docker image inspect --format"*) printf 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa';; esac
if [ "${2:-}" = 'sh -s' ]; then cat >/dev/null; fi
`)
	writeExecutable(t, root, "scp", "#!/bin/sh\nexit 0\n")
	cfg := Config{Project: Project{Name: "shop", Environment: "test"}, Hosts: map[string]Host{"host": {SSH: "fixture"}}, Services: map[string]Service{
		"db":  {Image: "fixture-db", Hosts: []string{"host"}, Phase: "infra"},
		"api": {Image: "fixture-api", Hosts: []string{"host"}, Phase: "backend"},
	}, Hooks: map[string][]Hook{"after_infra_healthy": {{Name: "storage ready", Run: []string{"sh", "-c", "echo INFRA_HOOK >> \"$PP_TEST_COMMAND_LOG\""}}}}}
	plan := BuildPlan(cfg, GitMetadata{SHA: "abcdef1", ShortSHA: "abcdef1"}, time.Now())
	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	if _, err := d.ApplyRelease(plan, bundle); err != nil {
		t.Fatal(err)
	}
	commands := string(mustRead(t, log))
	infra, hook, backend := strings.Index(commands, "--wait-timeout 300 'db'"), strings.Index(commands, "INFRA_HOOK"), strings.Index(commands, "--wait-timeout 300 'api'")
	if infra < 0 || hook < infra || backend < hook {
		t.Fatalf("wrong infrastructure order: %s", commands)
	}
}
