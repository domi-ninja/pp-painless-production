package deploy

import (
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

func retentionFixture(t *testing.T) (Deployer, Plan) {
	t.Helper()
	root := t.TempDir()
	d := NewDeployer(root, io.Discard, io.Discard)
	project := Project{Name: "retention", Environment: "test"}
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("release-%02d", i)
		status := "ok"
		if i > 8 {
			status = "failed"
		}
		bundle := filepath.Dir(releaseMetadataPath(root, project, id))
		r := ReleaseRecord{Project: project.Name, Environment: project.Environment, ReleaseID: id, BundlePath: bundle, Status: status, CreatedAt: time.Unix(int64(i), 0)}
		if err := SaveRelease(root, r); err != nil {
			t.Fatal(err)
		}
		writeNestedFile(t, bundle, "images/web.tar", "fake archive")
	}
	if err := SaveState(root, State{Project: project.Name, Environment: project.Environment, CurrentReleaseID: "release-02", PreviousReleaseID: "release-01"}); err != nil {
		t.Fatal(err)
	}
	return d, Plan{Config: Config{Project: project}}
}

func TestRetentionKeepsPinnedReleasesAndFiniteFailures(t *testing.T) {
	d, plan := retentionFixture(t)
	if err := d.cleanup(plan, true); err != nil {
		t.Fatal(err)
	}
	records, err := d.releaseRecords(plan.Config.Project)
	if err != nil || len(records) != 12 {
		t.Fatalf("dry-run modified releases: %d %v", len(records), err)
	}
	if _, err := os.Stat(filepath.Join(plan.Config.Project.localDir(d.Root), "cleanup.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote cleanup queue")
	}
	if err := d.cleanup(plan, false); err != nil {
		t.Fatal(err)
	}
	records, err = d.releaseRecords(plan.Config.Project)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, record := range records {
		ids = append(ids, record.ReleaseID)
	}
	if strings.Join(ids, ",") != "release-01,release-02,release-08,release-12" {
		t.Fatalf("retained %v", ids)
	}
	if err := d.cleanup(plan, false); err != nil {
		t.Fatal(err)
	}
}

func TestRetentionRetriesRemovedOfflineHostAndContinuesOthers(t *testing.T) {
	d, plan := retentionFixture(t)
	record, err := LoadRelease(d.Root, plan.Config.Project, "release-03")
	if err != nil {
		t.Fatal(err)
	}
	record.Hosts = []HostRecord{
		{ID: "old", SSH: "offline", RemoteDir: remoteReleaseDir(plan.Config.Project, record.ReleaseID)},
		{ID: "other", SSH: "online", RemoteDir: remoteReleaseDir(plan.Config.Project, record.ReleaseID)},
	}
	if err := SaveRelease(d.Root, record); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, d.Root, "ssh", `#!/bin/sh
cat >/dev/null
[ "$1" != offline ] || [ -f "$PP_TEST_ONLINE" ]
`)
	t.Setenv("PATH", d.Root+":"+os.Getenv("PATH"))
	t.Setenv("PP_TEST_ONLINE", filepath.Join(d.Root, "online"))
	if err := d.cleanup(plan, false); err == nil {
		t.Fatal("hid offline host")
	}
	queue, err := d.readCleanupQueue(plan.Config.Project)
	if err != nil || len(queue.Jobs) != 1 || len(queue.Jobs[0].Hosts) != 1 || queue.Jobs[0].Hosts[0].SSH != "offline" {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	writeFile(t, d.Root, "online", "")
	if err := d.cleanup(plan, false); err != nil {
		t.Fatal(err)
	}
	queue, err = d.readCleanupQueue(plan.Config.Project)
	if err != nil || len(queue.Jobs) != 0 {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	if _, err := os.Stat(record.BundlePath); !os.IsNotExist(err) {
		t.Fatal("did not remove expired local bundle")
	}
}

func TestRetentionRejectsSymlinksAndUnknownQueueVersions(t *testing.T) {
	for _, variant := range []string{"symlink", "queue"} {
		t.Run(variant, func(t *testing.T) {
			d, plan := retentionFixture(t)
			if variant == "symlink" {
				path := filepath.Dir(releaseMetadataPath(d.Root, plan.Config.Project, "release-03"))
				moved := filepath.Join(d.Root, "precious")
				if err := os.Rename(path, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, path); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := writeJSON(filepath.Join(plan.Config.Project.localDir(d.Root), "cleanup.json"), cleanupQueue{Version: 99}); err != nil {
					t.Fatal(err)
				}
			}
			if err := d.cleanup(plan, false); err == nil {
				t.Fatal("accepted unsafe cleanup")
			}
			if _, err := os.Stat(releaseMetadataPath(d.Root, plan.Config.Project, "release-04")); err != nil {
				t.Fatal("deleted before validation")
			}
		})
	}
}

func TestRetentionLegacyBundleCleanup(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	d := NewDeployer(root, io.Discard, io.Discard)
	if _, err := LoadState(root, project); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("old-%d", i)
		path := filepath.Join(root, ".deploy", "releases", id)
		if err := SaveRelease(root, ReleaseRecord{Project: project.Name, Environment: project.Environment, ResourceID: project.Name, ReleaseID: id, BundlePath: path, Status: "ok", CreatedAt: time.Unix(int64(i), 0)}); err != nil {
			t.Fatal(err)
		}
		writeNestedFile(t, path, "images/web.tar", "legacy archive")
	}
	if err := d.cleanup(Plan{Config: Config{Project: project}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".deploy", "releases", "old-0")); !os.IsNotExist(err) {
		t.Fatal("legacy archive retained")
	}
	if _, err := LoadRelease(root, project, "current"); err != nil {
		t.Fatal(err)
	}
}

func TestOperationLocksAndSpaceGuard(t *testing.T) {
	d := NewDeployer(t.TempDir(), io.Discard, io.Discard)
	unlock, err := d.operationLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.operationLock(); err == nil {
		t.Fatal("concurrent operation acquired lock")
	}
	unlock()
	unlock, err = d.operationLock()
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	writeExecutable(t, d.Root, "df", "#!/bin/sh\nprintf 'Filesystem 1024-blocks Used Available Capacity Mounted\\nx 100 99 1 99%% /\\n'\n")
	cmd := exec.Command("sh", "-c", spaceScript(".", 4096))
	cmd.Env = append(os.Environ(), "PATH="+d.Root+":"+os.Getenv("PATH"))
	if err := cmd.Run(); err == nil {
		t.Fatal("accepted insufficient free space")
	}
}

func TestRemoteCleanupKeepsActiveReleaseAndRejectsSymlink(t *testing.T) {
	for _, variant := range []string{"active", "symlink", "expired"} {
		t.Run(variant, func(t *testing.T) {
			root := t.TempDir()
			project := Project{Name: "shop", Environment: "prod"}
			host := HostRecord{SSH: "unused", RemoteDir: remoteReleaseDir(project, "old")}
			writeNestedFile(t, root, host.RemoteDir+"/env/app.env", "secret")
			writeExecutable(t, root, "ssh", "#!/bin/sh\nexec sh -c \"$2\"\n")
			writeExecutable(t, root, "docker", "#!/bin/sh\n[ \"$PP_TEST_ACTIVE\" != yes ] || echo container\n")
			t.Setenv("PATH", root+":"+os.Getenv("PATH"))
			if variant == "active" {
				t.Setenv("PP_TEST_ACTIVE", "yes")
			}
			if variant == "symlink" {
				path := filepath.Join(root, host.RemoteDir)
				if err := os.Rename(path, path+"-keep"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-keep", path); err != nil {
					t.Fatal(err)
				}
			}
			d := NewDeployer(root, io.Discard, io.Discard)
			err := d.cleanupRemoteRelease(project, "old", host)
			if (err != nil) != (variant != "expired") {
				t.Fatalf("cleanup: %v", err)
			}
			_, err = os.Stat(filepath.Join(root, host.RemoteDir, "env/app.env"))
			if os.IsNotExist(err) != (variant == "expired") {
				t.Fatalf("wrong deletion result: %v", err)
			}
		})
	}
}

func TestImagesForHostIncludesPinnedRollbackBuildOnly(t *testing.T) {
	root := t.TempDir()
	host := HostBundle{Path: filepath.Join(root, "hosts", "one"), Compose: filepath.Join(root, "hosts", "one", "compose.yml")}
	writeNestedFile(t, root, "hosts/one/compose.yml", "services:\n  web:\n    image: pp.local/shop/prod/web:old\n")
	id := "sha256:" + strings.Repeat("a", 64)
	for path, images := range map[string][]ManagedImage{
		filepath.Join(root, "images.json"):      {{Reference: "web:old", ID: id, Built: true}},
		filepath.Join(host.Path, "images.json"): {{Reference: "pp.local/shop/prod/web:old", ID: id}},
	} {
		if err := writeJSON(path, imageManifest{Version: 1, Images: images}); err != nil {
			t.Fatal(err)
		}
	}
	images, err := imagesForHost(host, []ImageBundle{{ID: "web", Tags: []string{"web:old"}}, {ID: "worker", Tags: []string{"worker:old"}}})
	if err != nil || len(images) != 1 || images[0].ID != "web" {
		t.Fatalf("images: %+v %v", images, err)
	}
}

func TestImageRemovalDoesNotDeleteRetaggedImageOrForce(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "log")
	writeExecutable(t, root, "docker", `#!/bin/sh
printf '%s\n' "$*" >> "$PP_TEST_LOG"
case "$*" in
 *'{{.Id}}'*) printf 'sha256:%064d\n' 2 ;;
 *'{{json .RepoTags}}'*) echo '["other:tag"]' ;;
 *'image rm'*) exit 99 ;;
esac
`)
	cmd := exec.Command("sh", "-c", imageRemovalScript([]ManagedImage{{Reference: "app:old", ID: "sha256:" + fmt.Sprintf("%064d", 1)}}))
	cmd.Env = append(os.Environ(), "PATH="+root+":"+os.Getenv("PATH"), "PP_TEST_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if bytes.Contains(mustRead(t, log), []byte("image rm")) {
		t.Fatal("deleted reassigned tag")
	}
}

func TestAutomaticRetentionAfterSuccessfulAndFailedDeploys(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	failed := filepath.Join(root, "fail-build")
	t.Setenv("PP_TEST_FAIL_BUILD", failed)
	for _, name := range []string{"docker", "ssh", "scp"} {
		writeExecutable(t, root, name, `#!/bin/sh
case "$*" in
 *pp-lock-ready*) printf 'pp-lock-ready\n'; cat >/dev/null; exit 0 ;;
 'buildx inspect '*--bootstrap) exit 0 ;;
 'buildx inspect '*) exit 1 ;;
 'buildx build '*) [ ! -f "$PP_TEST_FAIL_BUILD" ]; exit $? ;;
 *'image inspect --format '{{.Id}}*|*"image inspect --format '{{.Id}}'"*) printf 'sha256:%064d\n' 1; exit 0 ;;
 'info --format '*) echo /; exit 0 ;;
 *'{{.Size}}'*) echo 1024; exit 0 ;;
 'save -o '*) touch "$3" ;;
esac
if [ "$2" = 'sh -s' ]; then cat >/dev/null; fi
exit 0
`)
	}
	cfg := Config{Version: 1, Project: Project{Name: "shop", Environment: "test"}, Build: Build{Context: ".", Dockerfile: "Dockerfile"}, Hosts: map[string]Host{"host": {SSH: "unused"}}, Services: map[string]Service{"web": {Image: "shop:${git_sha}", Hosts: []string{"host"}}}}
	if err := writeYAML(filepath.Join(root, "deploy.yml"), cfg); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	git("init", "-q")
	git("add", "deploy.yml")
	var output bytes.Buffer
	d := NewDeployer(root, &output, &output)
	for i := 0; i < 10; i++ {
		git("-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", fmt.Sprintf("release %d", i))
		if i == 6 {
			writeFile(t, root, "fail-build", "")
		}
		err := d.Deploy()
		if (err != nil) != (i >= 6) {
			t.Fatalf("deploy %d: %v\n%s", i, err, output.String())
		}
	}
	records, err := d.releaseRecords(cfg.Project)
	if err != nil {
		t.Fatal(err)
	}
	successes, failures := 0, 0
	for _, record := range records {
		if record.Status == "ok" {
			successes++
		} else {
			failures++
		}
	}
	if successes != 3 || failures != 1 {
		t.Fatalf("kept %d successes, %d failures\n%s", successes, failures, output.String())
	}
	if err := d.Rollback(); err != nil {
		t.Fatalf("rollback after cleanup: %v\n%s", err, output.String())
	}
}

func TestLostHostLockCancelsWork(t *testing.T) {
	root := t.TempDir()
	writeExecutable(t, root, "ssh", "#!/bin/sh\nprintf 'pp-lock-ready\\n'\nexit 0\n")
	t.Setenv("PATH", root+":"+os.Getenv("PATH"))
	d := NewDeployer(root, io.Discard, io.Discard)
	plan := Plan{Config: Config{Project: Project{Name: "shop", Environment: "test"}}, Hosts: []HostPlan{{SSH: "unused"}}}
	unlock, err := d.lockHosts(plan, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	select {
	case <-d.Runner.context().Done():
	case <-time.After(2 * time.Second):
		t.Fatal("lost remote lock did not cancel operation")
	}
}

func TestCleanupDoesNotNeedBuildFilesSecretsOrGit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d, plan := retentionFixture(t)
	writeFile(t, d.Root, "deploy.yml", "project:\n  name: retention\n  environment: test\nenv:\n  source: missing-secret.env\n")
	if err := d.Cleanup(false); err != nil {
		t.Fatal(err)
	}
	records, err := d.releaseRecords(plan.Config.Project)
	if err != nil || len(records) != 4 {
		t.Fatalf("cleanup: %d %v", len(records), err)
	}
}

func TestRetentionValidation(t *testing.T) {
	for _, retention := range []Retention{{Releases: 1}, {Releases: -1}, {Failed: -1}, {MinFreeMB: -1}, {BuildCacheMB: -1}, {MinFreeMB: 1 << 50}, {BuildCacheMB: 1 << 50}} {
		if err := retention.validate(); err == nil {
			t.Fatalf("accepted %+v", retention)
		}
	}
	if err := (Retention{}).validate(); err != nil {
		t.Fatal(err)
	}
}
