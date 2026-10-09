package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitConfigCreatesStarterDeployYAML(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "My_App.Name")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := InitConfig(root, "deploy.yml")
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectName != "my-app-name" {
		t.Fatalf("unexpected project name %q", result.ProjectName)
	}

	body, err := os.ReadFile(filepath.Join(root, "deploy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, wanted := range []string{
		"version: 1",
		"name: my-app-name",
		"ssh: deploy@example.com",
		"published: auto",
		"host: my-app-name.example.com",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("starter config missing %q:\n%s", wanted, text)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gitignore), ".deploy/") {
		t.Fatalf(".gitignore missing .deploy/:\n%s", string(gitignore))
	}
	dockerignore, err := os.ReadFile(filepath.Join(root, ".dockerignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerignore), ".deploy") {
		t.Fatalf(".dockerignore missing .deploy:\n%s", string(dockerignore))
	}
}

func TestInitConfigRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "deploy.yml", "version: 1\n")

	_, err := InitConfig(root, "deploy.yml")
	if err == nil {
		t.Fatal("expected overwrite error")
	}
	if !strings.Contains(err.Error(), "deploy.yml already exists") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"My App":       "my-app",
		"__API..Prod":  "api-prod",
		"---":          "",
		"quotes_2026!": "quotes-2026",
	}
	for input, want := range cases {
		if got := slugify(input); got != want {
			t.Fatalf("slugify(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestStarterUsesLocalHealthAndKeepsSecretsOutOfBuildContext(t *testing.T) {
	root := t.TempDir()
	if _, err := InitConfig(root, "deploy.yml"); err != nil {
		t.Fatal(err)
	}
	body := string(mustRead(t, filepath.Join(root, "deploy.yml")))
	if !strings.Contains(body, "http://127.0.0.1:3000/health") || !strings.Contains(body, "${release}") || strings.Contains(body, "[\"true\"]") {
		t.Fatal("starter cannot check application readiness")
	}
	ignore := string(mustRead(t, filepath.Join(root, ".dockerignore")))
	for _, entry := range []string{".git\n", ".deploy\n", ".env\n", ".env.*\n"} {
		if !strings.Contains(ignore, entry) {
			t.Fatalf("missing ignore %s", entry)
		}
	}
}
