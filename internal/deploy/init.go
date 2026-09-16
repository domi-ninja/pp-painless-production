package deploy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type InitResult struct {
	Path        string
	ProjectName string
}

func InitConfig(root string, configPath string) (InitResult, error) {
	if err := validateRepoPath(root, configPath); err != nil {
		return InitResult{}, fmt.Errorf("config path: %w", err)
	}
	fullPath := filepath.Join(root, configPath)
	if _, err := os.Stat(fullPath); err == nil {
		return InitResult{}, fmt.Errorf("%s already exists", configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return InitResult{}, fmt.Errorf("check %s: %w", configPath, err)
	}

	projectName := slugify(filepath.Base(root))
	if projectName == "" {
		projectName = "app"
	}

	body := starterConfig(projectName)
	if err := os.WriteFile(fullPath, []byte(body), 0644); err != nil {
		return InitResult{}, fmt.Errorf("write %s: %w", configPath, err)
	}
	if err := ensureIgnoreEntry(root, ".gitignore", ".deploy/"); err != nil {
		return InitResult{}, err
	}
	if err := ensureIgnoreEntry(root, ".dockerignore", ".deploy"); err != nil {
		return InitResult{}, err
	}

	return InitResult{Path: fullPath, ProjectName: projectName}, nil
}

func starterConfig(projectName string) string {
	return fmt.Sprintf(`version: 1

project:
  name: %s
  environment: prod

build:
  context: .
  dockerfile: Dockerfile
  tags:
    - "%s:${git_sha}"

hosts:
  app-01:
    ssh: deploy@example.com
    roles: [web]

services:
  web:
    image: "%s:${git_sha}"
    hosts: [app-01]
    ports:
      - published: auto
        target: 3000
    health:
      command: ["true"]
      timeout_seconds: 60

routes:
  - host: %s.example.com
    service: web
`, projectName, projectName, projectName, projectName)
}

func slugify(value string) string {
	value = strings.ToLower(value)
	var out strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.':
			if out.Len() > 0 && !lastDash {
				out.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(out.String(), "-")
}

func ensureIgnoreEntry(root string, fileName string, entry string) error {
	path := filepath.Join(root, fileName)
	body, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read %s: %w", fileName, err)
	}
	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}

	prefix := ""
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		prefix = "\n"
	}
	addition := prefix + "\n# pp deployment bundles and state\n" + entry + "\n"
	if err := os.WriteFile(path, append(body, []byte(addition)...), 0644); err != nil {
		return fmt.Errorf("write %s: %w", fileName, err)
	}
	return nil
}
