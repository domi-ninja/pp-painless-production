package deploy

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in tests create only disposable, uniquely named Docker resources.
func TestDockerRetentionAndPinnedRollback(t *testing.T) {
	if os.Getenv("PP_TEST_DOCKER") != "1" {
		t.Skip("set PP_TEST_DOCKER=1 for real Docker retention checks")
	}
	root := t.TempDir()
	project := Project{Name: fmt.Sprintf("pp-test-%d", time.Now().UnixNano()), Environment: "test"}
	var resources []string
	var containers []string
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Cleanup(func() {
		for _, id := range containers {
			_ = exec.Command("docker", "rm", "-f", id).Run()
		}
		for i := len(resources) - 1; i >= 0; i-- {
			_ = exec.Command("docker", "image", "rm", resources[i]).Run()
		}
	})
	makeImage := func(tag, payload string) string {
		t.Helper()
		var archive bytes.Buffer
		w := tar.NewWriter(&archive)
		if err := w.WriteHeader(&tar.Header{Name: "payload", Mode: 0600, Size: int64(len(payload))}); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, payload)
		_ = w.Close()
		cmd := exec.Command("docker", "import", "--change", "LABEL pp.project="+project.Name, "--change", "LABEL pp.environment=test", "--change", "LABEL pp.release=old", "-", tag)
		cmd.Stdin = &archive
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("import: %v %s", err, out)
		}
		id := strings.TrimSpace(string(out))
		resources = append(resources, id, tag)
		return id
	}
	source := project.Name + ":latest"
	id := makeImage(source, "old")
	retained := managedImagePrefix(project) + "web:retained"
	run("tag", id, retained)
	resources = append(resources, retained)
	host := HostBundle{ID: "host", SSH: "unused", Path: filepath.Join(root, "bundle", "hosts", "host"), RemoteDir: remoteReleaseDir(project, "old")}
	host.Compose = filepath.Join(host.Path, "compose.yml")
	writeNestedFile(t, root, "bundle/hosts/host/compose.yml", "services:\n  web:\n    image: "+source+"\n    environment:\n      LITERAL: $$not_expanded\n")
	if err := os.MkdirAll(filepath.Join(root, host.RemoteDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(root, "bundle", "images.json"), imageManifest{Version: 1, Images: []ManagedImage{{Reference: source, ID: id, Built: true}}}); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "old.tar")
	run("save", "-o", archivePath, source)
	writeExecutable(t, root, "ssh", "#!/bin/sh\nexec sh -c \"$2\"\n")
	writeExecutable(t, root, "scp", "#!/bin/sh\ncp \"$1\" \"${2#*:}\"\n")
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	d := NewDeployer(root, io.Discard, io.Discard)
	plan := Plan{Config: Config{Project: project}, ReleaseID: "old"}
	if err := d.pinHostImages(plan, host); err != nil {
		t.Fatal(err)
	}
	alias := managedImagePrefix(project) + "web:old"
	resources = append(resources, alias)
	newID := makeImage(source, "new")
	if err := d.pinHostImages(plan, host); err != nil {
		t.Fatal(err)
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", alias); got != id {
		t.Fatalf("rollback image drifted: %s", got)
	}
	if !bytes.Contains(mustRead(t, host.Compose), []byte("$$not_expanded")) {
		t.Fatal("pinning changed literal environment")
	}
	manifest, err := readImageManifest(filepath.Join(host.Path, "images.json"))
	if err != nil {
		t.Fatal(err)
	}
	record := HostRecord{SSH: host.SSH, RemoteDir: host.RemoteDir, Images: manifest.Images}
	container := run("create", "--label", "pp.project="+project.Name, "--label", "pp.environment=test", "--label", "pp.release=old", "--entrypoint", "/unused", alias)
	containers = append(containers, container)
	if err := d.cleanupRemoteRelease(project, "old", record); err == nil {
		t.Fatal("deleted a release with a stopped container")
	}
	run("rm", container)
	if err := d.cleanupRemoteRelease(project, "old", record); err != nil {
		t.Fatal(err)
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", source); got != newID {
		t.Fatal("removed reassigned source tag")
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", retained); got != id {
		t.Fatal("removed shared retained image")
	}
	// The old archive can restore a built image after its remote tags are gone.
	run("image", "rm", retained)
	run("load", "-i", archivePath)
	if err := os.MkdirAll(filepath.Join(root, host.RemoteDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := d.pinHostImages(plan, host); err != nil {
		t.Fatal(err)
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", alias); got != id {
		t.Fatal("archive rollback did not restore exact image")
	}
	// A pp-owned source cache satisfies if_missing without hitting a registry.
	upstream := project.Name + "-upstream:v1"
	cache := sourceCacheReference(project, "web", upstream)
	run("tag", id, cache)
	resources = append(resources, upstream, cache)
	pullHost := HostBundle{ID: "pull", SSH: "unused", Path: filepath.Join(root, "pull-bundle", "hosts", "pull"), RemoteDir: remoteReleaseDir(project, "pull"), PullServices: []string{"web", "worker"}}
	pullHost.Compose = filepath.Join(pullHost.Path, "compose.yml")
	writeNestedFile(t, root, "pull-bundle/hosts/pull/compose.yml", "services:\n  web:\n    image: "+upstream+"\n  worker:\n    image: "+upstream+"\n")
	writeNestedFile(t, root, pullHost.RemoteDir+"/compose.yml", string(mustRead(t, pullHost.Compose)))
	pullPlan := Plan{Config: Config{Project: project, Services: map[string]Service{"web": {Pull: "if_missing"}, "worker": {Pull: "if_missing"}}}, ReleaseID: "pull"}
	if err := d.PullHostImages(pullPlan, pullHost); err != nil {
		t.Fatal(err)
	}
	pullAlias := managedImagePrefix(project) + "web:pull"
	resources = append(resources, pullAlias)
	resources = append(resources, managedImagePrefix(project)+"worker:pull")
	if err := d.pinHostImages(pullPlan, pullHost); err != nil {
		t.Fatal(err)
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", pullAlias); got != id {
		t.Fatal("if_missing cache changed image")
	}
	if err := exec.Command("docker", "image", "inspect", upstream).Run(); err == nil {
		t.Fatal("left temporary upstream source tag")
	}
	// A new upstream reference must not reuse the previous reference's cache.
	changed := project.Name + "-upstream:v2"
	pullHost.PullServices = []string{"web"}
	writeNestedFile(t, root, "pull-bundle/hosts/pull/compose.yml", "services:\n  web:\n    image: "+changed+"\n")
	if err := d.prepareSourceImages(pullPlan, pullHost); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("docker", "image", "inspect", changed).Run(); err == nil {
		t.Fatal("reused cache for a different upstream version")
	}
	// Local migration images are pinned separately from application builds.
	migrationPlan := Plan{Config: Config{Project: project}, ReleaseID: "migration"}
	migrationBundle, err := RenderBundle(root, migrationPlan)
	if err != nil {
		t.Fatal(err)
	}
	migrationID, err := d.pinMigrationImage(migrationPlan, source)
	if err != nil {
		t.Fatal(err)
	}
	migrationAlias := managedImagePrefix(project) + "migrations/runner:migration"
	resources = append(resources, migrationAlias)
	run("tag", newID, source)
	if pinned, err := d.pinMigrationImage(migrationPlan, source); err != nil || pinned != migrationID {
		t.Fatalf("migration pin changed: %s %v", pinned, err)
	}
	if err := d.cleanupLocalImages(project, ReleaseRecord{ReleaseID: "migration", BundlePath: migrationBundle.Root}, nil); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("docker", "image", "inspect", migrationAlias).Run(); err == nil {
		t.Fatal("expired local migration image alias retained")
	}
	if got := run("image", "inspect", "--format", "{{.Id}}", source); got != newID {
		t.Fatal("migration cleanup removed an unrelated source reference")
	}
}

func TestDockerDedicatedBuildCache(t *testing.T) {
	if os.Getenv("PP_TEST_DOCKER") != "1" {
		t.Skip("set PP_TEST_DOCKER=1 for real builder checks")
	}
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	project := Project{Name: fmt.Sprintf("pp-build-test-%d", time.Now().UnixNano()), Environment: "test"}
	writeFile(t, root, "Dockerfile", "FROM scratch\nCOPY payload /payload\n")
	writeFile(t, root, "payload", "bounded cache test")
	plan := Plan{Config: Config{Project: project, Retention: Retention{BuildCacheMB: 64, MinFreeMB: 1}, Build: Build{Context: ".", Dockerfile: "Dockerfile", Tags: []string{project.Name + ":test"}}}, ReleaseID: "test"}
	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	// Register cleanup before building, including failed builder creation/builds.
	t.Cleanup(func() {
		_ = exec.Command("docker", "image", "rm", project.Name+":test").Run()
		markers, _ := filepath.Glob(filepath.Join(os.Getenv("HOME"), ".pp", "*.owner"))
		for _, marker := range markers {
			_ = exec.Command("docker", "buildx", "rm", strings.TrimSuffix(filepath.Base(marker), ".owner")).Run()
		}
	})
	if err := d.BuildImages(plan, bundle); err != nil {
		t.Fatalf("build: %v\n%s", err, out.String())
	}
	manifest, err := readImageManifest(filepath.Join(bundle.Root, "images.json"))
	if err != nil || len(manifest.Images) != 1 {
		t.Fatalf("manifest: %+v %v", manifest, err)
	}
	if info, err := os.Stat(bundle.ImageTar); err != nil || info.Size() == 0 {
		t.Fatalf("archive: %v %v", info, err)
	}
	if err := d.BuildImages(plan, bundle); err != nil {
		t.Fatalf("reuse builder: %v\n%s", err, out.String())
	}
}
