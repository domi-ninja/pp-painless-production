package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHostPreflightRejectsOldComposeAndWrongPlatform(t *testing.T) {
	for _, test := range []struct {
		version, platform string
		blocked           bool
	}{{"2.30.0", "linux/amd64", false}, {"2.29.0", "linux/amd64", true}, {"2.30.0", "linux/arm64", true}} {
		t.Run(test.version+test.platform, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "pp")
			if err := os.Mkdir(base, 0755); err != nil {
				t.Fatal(err)
			}
			writeExecutable(t, root, "docker", "#!/bin/sh\nif [ \"$1\" = compose ]; then echo \"$PP_COMPOSE_VERSION\"; fi\n")
			writeExecutable(t, root, "uname", "#!/bin/sh\necho x86_64\n")
			cmd := exec.Command("sh", "-c", hostPreflightScript(base, false, []string{test.platform}))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_COMPOSE_VERSION="+test.version)
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.blocked {
				t.Fatalf("preflight: %v %s", err, out)
			}
		})
	}
}

func TestMissingOrWrongDataMountBlocksStartup(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "pp")
	writeNestedFile(t, root, "pp/required-mounts.json", `[{"path":"/data","source":"/dev/expected"}]`)
	writeExecutable(t, root, "findmnt", "#!/bin/sh\necho \"$PP_MOUNT_SOURCE\"\n")
	for _, source := range []string{"/dev/expected", "/dev/wrong", ""} {
		cmd := exec.Command("sh", "-c", requiredMountScript(base))
		cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_MOUNT_SOURCE="+source)
		out, err := cmd.CombinedOutput()
		if (err != nil) != (source != "/dev/expected") {
			t.Fatalf("mount %q: %v %s", source, err, out)
		}
	}
}
