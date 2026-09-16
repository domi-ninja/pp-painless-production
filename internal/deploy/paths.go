package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Resolve existing parents too, so missing files cannot hide an escaping symlink.
func validateRepoPath(root, path string) error {
	if path == "" || filepath.IsAbs(path) || strings.HasPrefix(path, "-") || strings.Contains(path, "://") || strings.ContainsAny(path, "\x00\r\n") {
		return fmt.Errorf("must be a relative path inside the application repository")
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return fmt.Errorf("must not contain parent-directory traversal")
		}
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	base, err = filepath.Abs(base)
	if err != nil {
		return err
	}
	candidate := filepath.Join(base, path)
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			rel, err := filepath.Rel(base, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("symlink resolves outside the application repository")
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// A dangling symlink is not an ordinary missing file.
		if info, statErr := os.Lstat(candidate); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains an unresolved symlink")
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return err
		}
		candidate = parent
	}
}
