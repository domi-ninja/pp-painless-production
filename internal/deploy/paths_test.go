package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryPathsRejectEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret", "/etc/passwd", "sub/../../secret", "escape/missing", "dangling", "dangling/missing", "https://example.com/build", "--help"} {
		if validateRepoPath(root, path) == nil {
			t.Errorf("accepted %q", path)
		}
	}
	for _, path := range []string{".", "Dockerfile", ".env.prod", "inside/missing", "sub/file"} {
		if err := validateRepoPath(root, path); err != nil {
			t.Errorf("%q: %v", path, err)
		}
	}
	if _, err := LoadConfig(root, "../secret"); err == nil {
		t.Fatal("loaded escaped config")
	}
	if _, err := InitConfig(root, "escape/deploy.yml"); err == nil {
		t.Fatal("wrote escaped config")
	}
}
