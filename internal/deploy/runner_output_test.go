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
	for _, test := range []struct {
		name, command, diagnostic string
	}{
		{"stderr", "printf 'failure detail' >&2; exit 7", "failure detail"},
		{"stdout", "printf 'partial result'; exit 7", "partial result"},
		{"both", "printf 'failure detail' >&2; printf 'partial result'; exit 7", "failure detail\npartial result"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (Runner{}).Output(t.TempDir(), "sh", "-c", test.command)
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 || !strings.HasSuffix(err.Error(), "\n"+test.diagnostic) {
				t.Fatalf("lost command diagnostic or exit status: %v", err)
			}
		})
	}
}
