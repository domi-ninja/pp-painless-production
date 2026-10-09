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

	for _, file := range []string{".gitignore", ".dockerignore"} {
		if err := noSymlinkPath(root, file); err != nil {
			return InitResult{}, err
		}
	}
	if err := ensureIgnoreEntries(root, ".gitignore", []string{".deploy/", ".env", ".env.*", "!.env.example", "!.env.*.example"}); err != nil {
		return InitResult{}, err
	}
	if err := ensureIgnoreEntries(root, ".dockerignore", []string{".git", ".deploy", ".env", ".env.*", "!.env.example", "!.env.*.example"}); err != nil {
		return InitResult{}, err
	}
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return InitResult{}, err
	}
	if _, err := file.WriteString(starterConfig(projectName)); err != nil {
		file.Close()
		return InitResult{}, err
	}
	if err := file.Close(); err != nil {
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
    - "%s:${release}"

hosts:
  app-01:
    ssh: deploy@example.com
    roles: [web]

services:
  web:
    image: "%s:${release}"
    hosts: [app-01]
    ports:
      - published: auto
        target: 3000
    health:
      http: http://127.0.0.1:3000/health
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

func ensureIgnoreEntry(root, fileName, entry string) error {
	return ensureIgnoreEntries(root, fileName, []string{entry})
}
func ensureIgnoreEntries(root, fileName string, entries []string) error {
	if err := noSymlinkPath(root, fileName); err != nil {
		return err
	}
	path := filepath.Join(root, fileName)
	body, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	existing := map[string]bool{}
	for _, line := range strings.Split(string(body), "\n") {
		existing[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, entry := range entries {
		if !existing[entry] {
			missing = append(missing, entry)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	prefix := "\n"
	if len(body) > 0 && !strings.HasSuffix(string(body), "\n") {
		prefix = "\n\n"
	}
	return os.WriteFile(path, append(body, []byte(prefix+"# pp\n"+strings.Join(missing, "\n")+"\n")...), 0644)
}
