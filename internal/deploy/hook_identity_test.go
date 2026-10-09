package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookReceivesSelectedIdentity(t *testing.T) {
	var out bytes.Buffer
	d := NewDeployer(t.TempDir(), &out, &out)
	d.ConfigPath = "deploy.dev.yml"
	plan := Plan{Config: Config{Project: Project{Name: "shop", Environment: "dev"}, Hosts: map[string]Host{"host": {SSH: "dev-host"}}, Hooks: map[string][]Hook{"after_backend_healthy": {{Name: "identity", Run: []string{"sh", "-c", `printf '%s:%s:%s:%s' "$PP_PROJECT" "$PP_ENVIRONMENT" "$PP_CONFIG" "$PP_HOST"`}}}}}}
	if err := d.RunHooks(plan, "after_backend_healthy"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "shop:dev:deploy.dev.yml:dev-host") {
		t.Fatal(out.String())
	}
}

func TestExampleSelectionRequiresOneMatchingEnvironment(t *testing.T) {
	script, err := filepath.Abs("../../examples/convex-website/scripts/pp-context.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"dev", "prod"} {
		t.Run(environment, func(t *testing.T) {
			root := t.TempDir()
			writeExecutable(t, root, "ssh", `#!/bin/sh
case "$*" in *label=pp.environment=dev*) echo dev-backend;; *label=pp.environment=prod*) echo prod-backend;; *) exit 9;; esac
`)
			cmd := exec.Command("bash", "-c", `source "$1"; pp_container convex-backend`, "probe", script)
			cmd.Env = append(os.Environ(), "PATH="+root+":"+os.Getenv("PATH"), "PP_HOST=selected-host", "PP_PROJECT=shop", "PP_ENVIRONMENT="+environment, "PP_ENV_FILE=.env.dev")
			out, err := cmd.CombinedOutput()
			if err != nil || strings.TrimSpace(string(out)) != environment+"-backend" {
				t.Fatalf("%s %v", out, err)
			}
			writeExecutable(t, root, "ssh", "#!/bin/sh\nprintf 'one\\ntwo\\n'\n")
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("accepted ambiguous containers: %s", out)
			}
		})
	}
}
