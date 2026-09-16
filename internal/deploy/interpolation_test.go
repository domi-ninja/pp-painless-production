package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposePreservesLiteralValues(t *testing.T) {
	if out, err := exec.Command("docker", "compose", "version").CombinedOutput(); err != nil {
		t.Skipf("requires Compose with raw env support: %s", out)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "env"), 0700); err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"DOLLARS":   "prefix${PP_UNSET}suffix$PP_AMBIENT$$",
		"QUOTES":    `it's "literal" # value`,
		"BACKSLASH": `line\nbackslash\path`,
		"SPACES":    " leading and trailing ",
	}
	// The source loader treats matching outer quotes as delimiters.
	writeFile(t, root, "source.env", "DOLLARS='"+values["DOLLARS"]+"'\nQUOTES="+values["QUOTES"]+"\nBACKSLASH="+values["BACKSLASH"]+"\nSPACES='"+values["SPACES"]+"'\n")
	path, err := renderServiceEnv(root, filepath.Join(root, "env"), "web", EnvSpec{Source: "source.env", IncludeAll: true})
	if err != nil {
		t.Fatal(err)
	}
	images, err := exec.Command("docker", "image", "ls", "-q").Output()
	if err != nil || len(strings.Fields(string(images))) == 0 {
		t.Skip("runtime inspection requires Docker and a cached image")
	}
	service, err := renderComposeService(Plan{}, "host", "web", Service{Image: strings.Fields(string(images))[0], Command: []string{"true"}, Environment: map[string]string{"INLINE": "${env.VALUE}"}, Health: Health{Command: []string{"printf", "${env.VALUE}"}}}, RenderVars{}, map[string]string{"VALUE": values["DOLLARS"]}, path)
	if err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(root, "compose.yml")
	if err := writeComposeYAML(compose, ComposeFile{Services: map[string]ComposeService{"web": service}}); err != nil {
		t.Fatal(err)
	}
	project := "literal-test-" + strings.ToLower(filepath.Base(root)) + "-" + strings.ToLower(filepath.Base(filepath.Dir(root)))
	args := []string{"compose", "-p", project, "-f", compose}
	t.Cleanup(func() {
		if out, err := exec.Command("docker", append(args, "down", "--remove-orphans")...).CombinedOutput(); err != nil {
			t.Errorf("cleanup: %v %s", err, out)
		}
	})
	cmd := exec.Command("docker", append(args, "create", "--pull", "never")...)
	cmd.Env = append(os.Environ(), "PP_AMBIENT=wrong", "PP_UNSET=wrong")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create: %v %s", err, out)
	}
	out, err := exec.Command("docker", "inspect", project+"-web-1").Output()
	if err != nil {
		t.Fatal(err)
	}
	var parsed []struct {
		Config struct {
			Env         []string
			Healthcheck struct{ Test []string }
		}
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	got := parsed[0].Config
	environment := map[string]string{}
	for _, entry := range got.Env {
		key, value, _ := strings.Cut(entry, "=")
		environment[key] = value
	}
	values["INLINE"] = values["DOLLARS"]
	for key, want := range values {
		if environment[key] != want {
			t.Errorf("%s: got %q, want %q", key, environment[key], want)
		}
	}
	if got.Healthcheck.Test[2] != values["DOLLARS"] {
		t.Fatalf("changed healthcheck argument: %q", got.Healthcheck.Test)
	}
}
