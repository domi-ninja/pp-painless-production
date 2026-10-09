package deploy

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryRevertsDatabaseBeforeContainers(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failed revert"}[fail], func(t *testing.T) {
			root := t.TempDir()
			project := legacyFixture(t, root)
			current, err := LoadRelease(root, project, "current")
			if err != nil {
				t.Fatal(err)
			}
			previous, err := LoadRelease(root, project, "previous")
			if err != nil {
				t.Fatal(err)
			}
			current.Migration = StepRecord{Status: "ok", Image: "sha256:" + strings.Repeat("a", 64)}
			writeNestedFile(t, root, mustRelative(t, root, filepath.Join(current.BundlePath, "migration.env")), "DB=old\n")
			if err := writeJSON(filepath.Join(current.BundlePath, "recovery.json"), recoveryContract{Version: 1, Migration: &Migration{RollbackCommand: []string{"revert"}, Env: EnvSpec{Source: "migration.env", IncludeAll: true}}}); err != nil {
				t.Fatal(err)
			}
			log := filepath.Join(root, "commands")
			t.Setenv("PP_TEST_COMMAND_LOG", log)
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			if fail {
				t.Setenv("PP_TEST_FAIL_REVERT", "1")
			}
			for _, name := range []string{"docker", "ssh", "scp"} {
				writeExecutable(t, root, name, `#!/bin/sh
printf '%s\n' "$*" >> "$PP_TEST_COMMAND_LOG"
case "$*" in 'start --attach'*) [ "${PP_TEST_FAIL_REVERT:-}" != 1 ] || exit 1;; esac
if [ "${2:-}" = 'sh -s' ]; then cat >/dev/null; fi
`)
			}
			var out bytes.Buffer
			d := NewDeployer(root, &out, &out)
			step, _, err := d.recoverRelease(current, &previous, true)
			commands := string(mustRead(t, log))
			revert := strings.Index(commands, "start --attach")
			apply := strings.Index(commands, "compose -f")
			if revert < 0 {
				t.Fatal("no database revert")
			}
			if fail {
				if err == nil || step.Status != "failed" || apply >= 0 {
					t.Fatalf("started older code after failed revert: %v %s", err, commands)
				}
			} else if err != nil || apply < revert {
				t.Fatalf("wrong order: %v %s", err, commands)
			}
		})
	}
}

func TestHTTPChecksRetryAndCancel(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	d := NewDeployer(t.TempDir(), &out, &out)
	if _, err := d.RunCheck(Plan{}, "smoke", Check{Name: "retry", URL: server.URL}); err != nil || calls.Load() != 2 {
		t.Fatalf("retry: %d %v", calls.Load(), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.Runner.Context = ctx
	start := time.Now()
	if _, err := d.RunCheck(Plan{}, "smoke", Check{Name: "cancel", URL: server.URL}); err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := LoadPlanContext(ctx, t.TempDir(), "deploy.yml"); err == nil {
		t.Fatal("accepted missing config")
	}
}
