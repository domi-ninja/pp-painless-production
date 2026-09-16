package deploy

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadProtectsNewAndExistingSecretFiles(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, root, "ssh", "#!/bin/sh\nexec sh -c \"$2\"\n")
	writeExecutable(t, root, "scp", "#!/bin/sh\ncp \"$1\" \"${2#*:}\"\nchmod 0644 \"${2#*:}\"\n")
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	compose := filepath.Join(root, "compose.yml")
	if err := writeYAML(compose, map[string]string{"password": "test"}); err != nil {
		t.Fatal(err)
	}
	assertMode(t, compose, 0600)
	writeFile(t, root, "app.env", "PASSWORD=test\n")
	host := HostBundle{SSH: "unused", Compose: compose, EnvFiles: []string{filepath.Join(root, "app.env")}, RemoteDir: filepath.Join(root, "remote")}
	d := NewDeployer(root, io.Discard, io.Discard)
	for i := 0; i < 2; i++ {
		if err := d.UploadHostBundle(host, nil); err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{host.RemoteDir, host.RemoteDir + "/env", host.RemoteDir + "/images"} {
			assertMode(t, dir, 0700)
			if err := os.Chmod(dir, 0755); err != nil {
				t.Fatal(err)
			}
		}
		for _, file := range []string{host.RemoteDir + "/compose.yml", host.RemoteDir + "/env/app.env"} {
			assertMode(t, file, 0600)
			if err := os.Chmod(file, 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s: mode %o, want %o", path, info.Mode().Perm(), want)
	}
}
