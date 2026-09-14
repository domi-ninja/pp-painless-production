package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullPolicies(t *testing.T) {
	for _, test := range []struct {
		name, policy                 string
		cached, denied, pulled, fail bool
	}{
		{name: "cached latest works offline", policy: "if_missing", cached: true, denied: true},
		{name: "missing image pulls", policy: "if_missing", pulled: true},
		{name: "missing image denial fails", policy: "if_missing", denied: true, pulled: true, fail: true},
		{name: "always refreshes cached image", policy: "always", cached: true, pulled: true},
		{name: "always does not hide denial", policy: "always", cached: true, denied: true, pulled: true, fail: true},
		{name: "never does not pull", policy: "never", denied: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			log := filepath.Join(root, "commands")
			writeExecutable(t, root, "docker", `#!/bin/sh
printf '%s\n' "$*" >> "$PP_TEST_COMMAND_LOG"
case "$*" in
  'compose -f compose.yml -p shop_prod config --images s3') echo minio/minio:latest ;;
  'image inspect minio/minio:latest') [ "$PP_TEST_CACHED" = true ] ;;
  'compose -f compose.yml -p shop_prod pull --policy always s3') [ "$PP_TEST_DENIED" != true ] ;;
  *) exit 99 ;;
esac
`)
			plan := Plan{Config: Config{Project: Project{Name: "shop", Environment: "prod"}, Services: map[string]Service{"s3": {Pull: test.policy}}}}
			host := HostBundle{RemoteDir: root, PullServices: []string{"s3"}}
			cmd := exec.Command("sh", "-c", pullHostImagesScript(plan, host))
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_TEST_COMMAND_LOG="+log)
			if test.cached {
				cmd.Env = append(cmd.Env, "PP_TEST_CACHED=true")
			}
			if test.denied {
				cmd.Env = append(cmd.Env, "PP_TEST_DENIED=true")
			}
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.fail {
				t.Fatalf("err=%v, output=%s", err, out)
			}
			body, err := os.ReadFile(log)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			pulls := strings.Count(string(body), "pull --policy always")
			if (pulls == 1) != test.pulled || pulls > 1 {
				t.Fatalf("unexpected pulls: %s", body)
			}
		})
	}
}
