package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretInputsAreScopedAndErrorsDoNotPrintArguments(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "env", "ALLOWED=value\nOTHER=synthetic-secret\n")
	values, err := loadSelectedEnv(root, EnvSpec{Source: "env", Required: []string{"ALLOWED"}})
	if err != nil || len(values) != 1 {
		t.Fatalf("%v %v", values, err)
	}
	var out bytes.Buffer
	r := Runner{Stdout: &out, Stderr: &out}
	err = r.Run(root, "sh", "-c", "exit 1", "synthetic-secret")
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal(err)
	}
	if safeCheckURL("https://example.com/health?token=synthetic-secret") != "https://example.com/health" {
		t.Fatal("query exposed")
	}
}

func TestInspectionAndInitRejectSymlinkDestinations(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".deploy")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(root, Project{Name: "shop", Environment: "prod"}); err == nil {
		t.Fatal("accepted state symlink")
	}
	if err := os.Symlink(filepath.Join(outside, "ignore"), filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := ensureIgnoreEntry(root, ".gitignore", ".deploy/"); err == nil {
		t.Fatal("accepted ignore symlink")
	}
}
