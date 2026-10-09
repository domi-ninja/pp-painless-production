package deploy

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusShowsFailedFirstAttemptAndObservedDrift(t *testing.T) {
	root := t.TempDir()
	project := Project{Name: "shop", Environment: "test"}
	writeFile(t, root, "deploy.yml", "project:\n  name: shop\n  environment: test\n")
	record := ReleaseRecord{Project: project.Name, Environment: project.Environment, ReleaseID: "failed", BundlePath: filepath.Dir(releaseMetadataPath(root, project, "failed")), Hosts: []HostRecord{{ID: "app", SSH: "unused", RemoteDir: remoteReleaseDir(project, "failed"), Services: []string{"web"}}}}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	state := State{}
	finish, err := d.trackAttempt(&state, &record, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.recordHostProgress(&record)("app", "healthy: web"); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("second host failed")
	finish(&failure)
	if failure == nil {
		t.Fatal("lost failure")
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, root, "ssh", "#!/bin/sh\nprintf 'web\\tfailed\\trunning\\tUp 1 minute (healthy)\\n'\n")
	if err := d.Status(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"latest attempt: deploy failed (failed)", "local current: -", "app: applied healthy: web", "observed host app", "differs from local"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, out.String())
		}
	}
	out.Reset()
	writeExecutable(t, root, "ssh", "#!/bin/sh\nexit 1\n")
	if err := d.Status(); err == nil || !strings.Contains(out.String(), "unreachable") {
		t.Fatalf("unreachable host: %v %s", err, out.String())
	}
}

func TestAttemptReportsPersistenceFailure(t *testing.T) {
	root := t.TempDir()
	project := Project{Name: "shop", Environment: "test"}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	record := ReleaseRecord{Project: project.Name, Environment: project.Environment, ReleaseID: "release", BundlePath: filepath.Dir(releaseMetadataPath(root, project, "release"))}
	state := State{}
	finish, err := d.trackAttempt(&state, &record, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	path := statePath(root, project)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	var result error
	finish(&result)
	if result == nil {
		t.Fatal("discarded state persistence error")
	}
}

func TestFailedRollbackKeepsCurrentAndRecordsAttempt(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	writeFile(t, root, "deploy.yml", "project:\n  name: shop\n  environment: prod\n")
	previous, err := LoadRelease(root, project, "previous")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(previous.BundlePath, "recovery.json"), recoveryContract{Version: 1, HasHooks: true}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	if err := d.Rollback(); err == nil {
		t.Fatal("accepted unsafe rollback")
	}
	state, err := LoadState(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if state.CurrentReleaseID != "current" || state.LatestAttempt == nil || state.LatestAttempt.Action != "rollback" || state.LatestAttempt.Step.Status != "failed" {
		t.Fatalf("lost rollback attempt: %+v", state)
	}
}
