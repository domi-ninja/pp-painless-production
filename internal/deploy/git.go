package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"strings"
	"time"
)

type GitMetadata struct {
	SHA        string
	ShortSHA   string
	Dirty      bool
	DiffDigest string
}

func ReadGitMetadata(root string) (GitMetadata, error) {
	return readGitMetadataContext(context.Background(), root)
}

func readGitMetadataContext(ctx context.Context, root string) (GitMetadata, error) {
	sha, err := gitOutputContext(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return GitMetadata{}, err
	}
	shortSHA, err := gitOutputContext(ctx, root, "rev-parse", "--short=7", "HEAD")
	if err != nil {
		return GitMetadata{}, err
	}

	status, err := gitOutputContext(ctx, root, "status", "--porcelain")
	if err != nil {
		return GitMetadata{}, err
	}

	diff, err := gitOutputContext(ctx, root, "diff", "HEAD")
	if err != nil {
		return GitMetadata{}, err
	}
	sum := sha256.Sum256([]byte(status + "\n" + diff))

	return GitMetadata{
		SHA:        sha,
		ShortSHA:   shortSHA,
		Dirty:      strings.TrimSpace(status) != "",
		DiffDigest: hex.EncodeToString(sum[:]),
	}, nil
}

func ReleaseID(now time.Time, shortSHA string) string {
	return now.UTC().Format("20060102T150405Z") + "-" + shortSHA
}

func gitOutput(root string, args ...string) (string, error) {
	return gitOutputContext(context.Background(), root, args...)
}
func gitOutputContext(ctx context.Context, root string, args ...string) (string, error) {
	gitArgs := append([]string{"-c", "safe.directory=" + root}, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
