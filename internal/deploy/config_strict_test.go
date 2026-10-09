package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRejectsUnknownNestedFieldsAndExtraDocuments(t *testing.T) {
	root := t.TempDir()
	for _, body := range []string{"version: 1\npriviliged: true\n", "services:\n  web:\n    security_opt: [no-new-privileges]\n", "hosts:\n  app:\n    policy: private\n", "project:\n  name: shop\n  environment: prod\n---\nproject: {}\n"} {
		writeFile(t, root, "deploy.yml", body)
		if _, err := LoadConfig(root, "deploy.yml"); err == nil || (!strings.Contains(err.Error(), "field ") && !strings.Contains(err.Error(), "one YAML document")) {
			t.Fatalf("accepted unknown configuration: %v", err)
		}
	}
}

func TestConfigRejectsUnsupportedAndInvalidInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"phase", func(c *Config) { s := c.Services["web"]; s.Phase = "database"; c.Services["web"] = s }, "phase must be"},
		{"hook", func(c *Config) { c.Hooks = map[string][]Hook{"before_build": {}} }, "unsupported hook"},
		{"check", func(c *Config) { c.Checks = map[string][]Check{"integration": {}} }, "unsupported check"},
		{"platforms", func(c *Config) { c.Build.Platforms = []string{"linux/amd64", "linux/arm64"} }, "one platform"},
		{"port", func(c *Config) { s := c.Services["web"]; s.Ports[0].Target = 70000; c.Services["web"] = s }, "65535"},
		{"host IP", func(c *Config) { s := c.Services["web"]; s.Ports[0].HostIP = "localhost"; c.Services["web"] = s }, "IP address"},
		{"timeout", func(c *Config) { c.Migrations.TimeoutSeconds = -1 }, "timeout_seconds"},
		{"CI placement", func(c *Config) { h := c.Hosts["app-01"]; h.Roles = []string{"ci"}; c.Hosts["app-01"] = h }, "cannot place"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "Dockerfile", "FROM scratch\n")
			writeFile(t, root, ".env.prod", "DATABASE_URL=db\nSESSION_SECRET=secret\nQUEUE_URL=queue\n")
			cfg := validConfig()
			test.change(&cfg)
			if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("invalid config: %v", err)
			}
		})
	}
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=\nSESSION_SECRET=secret\nQUEUE_URL=queue\n")
	cfg := validConfig()
	if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), "missing or empty DATABASE_URL") {
		t.Fatalf("empty secret: %v", err)
	}
	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	if err := os.Remove(filepath.Join(root, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), "build.dockerfile") {
		t.Fatalf("missing build file: %v", err)
	}
}
