package deploy

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadEnvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s does not exist", path)
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d must be KEY=VALUE", path, lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !envKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("%s:%d has an invalid env key", path, lineNumber)
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("%s:%d repeats env key %s", path, lineNumber, key)
		}
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') && (len(value) < 2 || value[len(value)-1] != value[0]) {
			return nil, fmt.Errorf("%s:%d has an unmatched env quote", path, lineNumber)
		}
		values[key] = trimEnvQuotes(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return values, nil
}

func trimEnvQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

// Load only declared file values; operator tool settings remain in the process environment.
func loadSelectedEnv(root string, spec EnvSpec) (map[string]string, error) {
	if spec.Source == "" {
		return map[string]string{}, nil
	}
	values, err := LoadEnvFile(filepath.Join(root, spec.Source))
	if err != nil {
		return nil, err
	}
	if len(spec.Required) == 0 && !spec.IncludeAll {
		return nil, fmt.Errorf("env.required must list injected keys, or set include_all: true")
	}
	selected := map[string]string{}
	for _, key := range spec.Required {
		value, ok := values[key]
		if !ok || value == "" {
			return nil, fmt.Errorf("missing or empty required env key %s", key)
		}
		selected[key] = value
	}
	if spec.IncludeAll {
		return values, nil
	}
	return selected, nil
}

func encodeEnv(values map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var body strings.Builder
	for _, key := range keys {
		if !envKeyPattern.MatchString(key) || strings.ContainsAny(values[key], "\r\n") {
			return nil, fmt.Errorf("unsupported multiline env key or value")
		}
		body.WriteString(key + "=" + values[key] + "\n")
	}
	return []byte(body.String()), nil
}

// Saved inputs are read by the pp parser, so protect literal outer quotes and spaces.
func encodeSourceEnv(values map[string]string) ([]byte, error) {
	if _, err := encodeEnv(values); err != nil {
		return nil, err
	}
	var body strings.Builder
	for _, item := range sortedMap(values) {
		body.WriteString(item.Key + "='" + item.Value + "'\n")
	}
	return []byte(body.String()), nil
}
