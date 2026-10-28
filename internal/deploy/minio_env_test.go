package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMinioBucketLookupSelectsEnvironment(t *testing.T) {
	for _, override := range []string{"", "dev"} {
		root := t.TempDir()
		writeFile(t, root, "deploy.yml", "project:\n  name: app\n  environment: prod\n")
		writeFile(t, root, ".env.local", "S3_STORAGE_EXPORTS_BUCKET=exports\nS3_STORAGE_SNAPSHOT_IMPORTS_BUCKET=imports\nS3_STORAGE_MODULES_BUCKET=modules\nS3_STORAGE_FILES_BUCKET=files\nS3_STORAGE_SEARCH_BUCKET=search\n")
		writeExecutable(t, root, "ssh", `#!/bin/sh
exec bash -c "$2"
`)
		writeExecutable(t, root, "docker", `#!/bin/sh
if [ "$1" = ps ]; then
 printf '%s\n' "$@" > "$PP_TEST_FILTERS"
 exit 0
fi
exit 99
`)
		script, err := filepath.Abs(filepath.Join("..", "..", "examples", "convex-website", "scripts", "ensure-minio-buckets.sh"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("bash", script)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_HOST=unused", "PP_PROJECT=app", "PP_ENVIRONMENT="+override, "PP_TEST_FILTERS="+filepath.Join(root, "filters"))
		if err := cmd.Run(); err == nil {
			t.Fatal("expected empty fake container lookup to stop execution")
		}
		want := override
		if want == "" {
			want = "prod"
		}
		if !strings.Contains(string(mustRead(t, filepath.Join(root, "filters"))), "label=pp.environment="+want) {
			t.Fatal("missing intended environment filter")
		}
	}
}
