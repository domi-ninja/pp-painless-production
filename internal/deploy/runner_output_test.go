package deploy

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestRunnerOutputSeparatesDiagnostics(t *testing.T) {
	out, err := (Runner{}).Output(t.TempDir(), "sh", "-c", "printf 'managed\n'; printf 'SSH warning\n' >&2")
	if err != nil || out != "managed" {
		t.Fatalf("output=%q err=%v", out, err)
	}
	_, err = (Runner{}).Output(t.TempDir(), "sh", "-c", "printf 'failure detail' >&2; exit 7")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 || !strings.Contains(err.Error(), "failure detail") {
		t.Fatalf("lost command diagnostic or exit status: %v", err)
	}
}
