package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.domi.ninja/domi-ninja/infra-meta-forgejo/internal/deploy"
)

func TestConfigFlagSelectsEnvironmentState(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
	var out bytes.Buffer
	if code := Main("pp", []string{"init", "--config", "deploy.dev.yml"}, &out, &out); code != 0 {
		t.Fatalf("%d: %s", code, out.String())
	}
	path := filepath.Join(root, "deploy.dev.yml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.ReplaceAll(body, []byte("environment: prod"), []byte("environment: dev")), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := deploy.LoadConfig(root, "deploy.dev.yml")
	if err != nil {
		t.Fatal(err)
	}
	// A production release must not appear in development status.
	if err := deploy.SaveState(root, deploy.State{Project: cfg.Project.Name, Environment: "prod", CurrentReleaseID: "production"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Main("pp", []string{"status", "--config", "deploy.dev.yml"}, &out, &out); code != 0 || !strings.Contains(out.String(), "no deployments recorded") {
		t.Fatalf("%d: %s", code, out.String())
	}
}
