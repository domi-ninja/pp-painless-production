package deploy

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestDockerMigrationTimeoutAndExitCleanup(t *testing.T) {
	if os.Getenv("PP_TEST_DOCKER") != "1" {
		t.Skip("set PP_TEST_DOCKER=1 for isolated migration container checks")
	}
	image := "debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587"
	if err := exec.Command("docker", "image", "inspect", image).Run(); err != nil {
		t.Skip("pull the pinned Debian test image first")
	}
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed exit", true: "timeout"}[timeout], func(t *testing.T) {
			var out bytes.Buffer
			d := NewDeployer(t.TempDir(), &out, &out)
			command := []string{"sh", "-c", "exit 7"}
			if timeout {
				command = []string{"sleep", "30"}
			}
			err := d.runMigration(&Migration{Command: command, TimeoutSeconds: 1}, image, false)
			if err == nil || (timeout && !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatalf("migration error: %v", err)
			}
			lines := strings.Fields(out.String())
			if len(lines) == 0 || !imageIDPattern.MatchString("sha256:"+lines[0]) {
				t.Fatalf("missing created container ID: %s", out.String())
			}
			id := lines[0]
			t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
			if err := exec.Command("docker", "inspect", id).Run(); err == nil {
				t.Fatal("owned migration container survived")
			}
		})
	}
}

func TestRecoveryDoesNotUseUndoToMoveToNewerDatabaseRelease(t *testing.T) {
	var out bytes.Buffer
	d := NewDeployer(t.TempDir(), &out, &out)
	current := ReleaseRecord{CreatedAt: time.Now(), Migration: StepRecord{Status: "ok"}}
	target := ReleaseRecord{CreatedAt: current.CreatedAt.Add(time.Minute)}
	if _, _, err := d.recoverRelease(current, &target, true); err == nil || !strings.Contains(err.Error(), "target release is newer") {
		t.Fatalf("guessed forward database recovery: %v", err)
	}
}
