package deploy

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestGitOutputPreservesDiagnostic(t *testing.T) {
	_, err := gitOutput(t.TempDir(), "rev-parse", "HEAD")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("missing git diagnostic or wrapped exit error: %v", err)
	}
}
