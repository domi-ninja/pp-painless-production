package deploy

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationUsesParsedPrivateEnvironment(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		for _, required := range [][]string{nil, {"DATABASE_URL"}} {
			root := t.TempDir()
			writeFile(t, root, "source.env", "DATABASE_URL=\"postgres://example\"\nOTHER=other\n")
			writeExecutable(t, root, "docker", `#!/bin/sh
while [ "$#" -gt 0 ]; do
 if [ "$1" = --env-file ]; then
  shift
  printf '%s' "$1" > "$PP_TEST_ENV_PATH"
  stat -c '%a' "$1" > "$PP_TEST_ENV_MODE"
  cat "$1" > "$PP_TEST_ENV_CONTENT"
 fi
 shift
done
`)
			t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("PP_TEST_ENV_PATH", filepath.Join(root, "path"))
			t.Setenv("PP_TEST_ENV_MODE", filepath.Join(root, "mode"))
			t.Setenv("PP_TEST_ENV_CONTENT", filepath.Join(root, "content"))
			d := NewDeployer(root, io.Discard, io.Discard)
			err := d.runMigration(&Migration{Command: []string{"up"}, RollbackCommand: []string{"down"}, Env: EnvSpec{Source: "source.env", Required: required}}, "test-image", rollback)
			if err != nil {
				t.Fatal(err)
			}
			want := "DATABASE_URL=postgres://example\n"
			if len(required) == 0 {
				want += "OTHER=other\n"
			}
			if got := string(mustRead(t, filepath.Join(root, "content"))); got != want {
				t.Fatalf("env=%q, want=%q", got, want)
			}
			if got := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "mode")))); got != "600" {
				t.Fatalf("mode=%s", got)
			}
			envPath := string(mustRead(t, filepath.Join(root, "path")))
			if _, err := os.Stat(envPath); !os.IsNotExist(err) {
				t.Fatalf("temporary env not removed: %v", err)
			}
		}
	}
}
