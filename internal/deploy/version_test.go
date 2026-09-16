package deploy

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func legacyFixture(t *testing.T, root string) Project {
	t.Helper()
	project := Project{Name: "shop", Environment: "prod"}
	for _, id := range []string{"previous", "current"} {
		bundle := filepath.Join(root, ".deploy", "releases", id)
		record := ReleaseRecord{
			Project: project.Name, Environment: project.Environment, ReleaseID: id,
			BundlePath: bundle, RemoteBase: ".pp/shop", ImageTar: filepath.Join(bundle, "images", "web.tar"),
			Hosts: []HostRecord{{ID: "host", SSH: "deploy@example", RemoteDir: ".pp/shop/releases/" + id}},
		}
		if id == "current" {
			record.PreviousReleaseID = "previous"
		}
		writeNestedFile(t, root, filepath.Join(".deploy", "releases", id, "hosts", "host", "compose.yml"), "services: {}\n")
		if err := writeJSON(filepath.Join(bundle, "metadata.json"), record); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeJSON(filepath.Join(root, ".deploy", "state.json"), State{
		Project: "shop", Environment: "prod", CurrentReleaseID: "current", PreviousReleaseID: "previous",
	}); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestLegacyStateMigrationPreservesResourcesAndHistory(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	original := mustRead(t, filepath.Join(root, ".deploy", "state.json"))
	metadata := mustRead(t, filepath.Join(root, ".deploy", "releases", "current", "metadata.json"))
	for i := 0; i < 2; i++ {
		state, err := LoadState(root, project)
		if err != nil {
			t.Fatal(err)
		}
		if state.Version != 2 || state.ResourceID != "shop" || state.CurrentReleaseID != "current" || state.PreviousReleaseID != "previous" {
			t.Fatalf("bad migrated state: %+v", state)
		}
		for _, id := range []string{"current", "previous"} {
			record, err := LoadRelease(root, project, id)
			if err != nil {
				t.Fatal(err)
			}
			plan := planForRecord(Plan{Config: Config{Project: project}}, record)
			bundle := bundleFromRecord(record)
			if record.Version != 2 || plan.Config.Project.ResourceID() != "shop" || plan.Config.Project.routePath() != "/etc/pp/proxy/routes/shop.caddy" || bundle.Hosts[0].RemoteDir != ".pp/shop/releases/"+id {
				t.Fatalf("changed runtime identity: %+v", record)
			}
			if _, err := os.Stat(bundle.Hosts[0].Compose); err != nil {
				t.Fatalf("lost rollback compose: %v", err)
			}
		}
	}
	if !bytes.Equal(original, mustRead(t, filepath.Join(root, ".deploy", "state.json"))) || !bytes.Equal(metadata, mustRead(t, filepath.Join(root, ".deploy", "releases", "current", "metadata.json"))) {
		t.Fatal("modified original state or release")
	}
	dev := Project{Name: "shop", Environment: "dev"}
	state, err := LoadState(root, dev)
	if err != nil || state.CurrentReleaseID != "" || dev.ResourceID() != "shop_dev" {
		t.Fatalf("dev adopted prod: %+v, %v", state, err)
	}
	if _, err := LoadRelease(root, dev, "current"); err == nil {
		t.Fatal("dev loaded prod release")
	}
}

func TestMigrationFailureDoesNotPublishPartialState(t *testing.T) {
	for _, change := range []string{"missing previous", "foreign current", "future release", "wrong path"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			project := legacyFixture(t, root)
			path := filepath.Join(root, ".deploy", "releases", "current", "metadata.json")
			var record ReleaseRecord
			if err := json.Unmarshal(mustRead(t, path), &record); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "missing previous":
				if err := os.Remove(filepath.Join(root, ".deploy", "releases", "previous", "metadata.json")); err != nil {
					t.Fatal(err)
				}
			case "foreign current":
				record.Environment = "dev"
			case "future release":
				record.Version = 99
			case "wrong path":
				record.BundlePath = root
			}
			if err := writeJSON(path, record); err != nil {
				t.Fatal(err)
			}
			original := mustRead(t, filepath.Join(root, ".deploy", "state.json"))
			if _, err := LoadState(root, project); err == nil {
				t.Fatal("accepted incomplete or invalid migration")
			}
			if _, err := os.Stat(project.localDir(root)); !os.IsNotExist(err) {
				t.Fatal("published partial migration")
			}
			if !bytes.Equal(original, mustRead(t, filepath.Join(root, ".deploy", "state.json"))) {
				t.Fatal("changed original state")
			}
		})
	}
}

func TestScopedV1UpgradeAndFutureVersionRejection(t *testing.T) {
	root := t.TempDir()
	project := Project{Name: "shop", Environment: "prod"}
	path := statePath(root, project)
	writeNestedFile(t, root, filepath.Join(".deploy", "shop", "prod", "state.json"), `{"project":"shop","environment":"prod","current_release_id":"release"}`)
	original := mustRead(t, path)
	state, err := LoadState(root, project)
	if err != nil || state.Version != 2 || state.ResourceID != "shop_prod" {
		t.Fatalf("bad upgrade: %+v, %v", state, err)
	}
	if !bytes.Equal(original, mustRead(t, path+".v1.bak")) {
		t.Fatal("backup differs")
	}
	recordPath := releaseMetadataPath(root, project, "release")
	if err := os.MkdirAll(filepath.Dir(recordPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(recordPath, ReleaseRecord{Project: "shop", Environment: "prod", ReleaseID: "release", BundlePath: filepath.Dir(recordPath)}); err != nil {
		t.Fatal(err)
	}
	if record, err := LoadRelease(root, project, "release"); err != nil || record.Version != 2 || record.ResourceID != "shop_prod" {
		t.Fatalf("release upgrade: %+v, %v", record, err)
	}
	if _, err := os.Stat(recordPath + ".v1.bak"); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{-1, 99} {
		state.Version = version
		if err := writeJSON(path, state); err != nil {
			t.Fatal(err)
		}
		original := mustRead(t, path)
		if _, err := LoadState(root, project); err == nil || !strings.Contains(err.Error(), "unsupported state version") {
			t.Fatalf("accepted version %d: %v", version, err)
		}
		if !bytes.Equal(original, mustRead(t, path)) {
			t.Fatal("rewrote unsupported state")
		}
	}
}

func TestConcurrentLegacyMigration(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := LoadState(root, project); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
}

func TestAdoptedDeployAndRollbackKeepResourceIdentity(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	cfg := Config{
		Version: 1, Project: project, Build: Build{Context: ".", Dockerfile: "Dockerfile"},
		Hosts:    map[string]Host{"host": {SSH: "unused"}},
		Services: map[string]Service{"web": {Image: "shop:latest", Hosts: []string{"host"}}},
	}
	if err := writeYAML(filepath.Join(root, "deploy.yml"), cfg); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "deploy.yml"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	log := filepath.Join(root, "commands")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PP_TEST_COMMAND_LOG", log)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"docker", "scp", "ssh"} {
		writeExecutable(t, root, name, `#!/bin/sh
printf '%s\n' "$*" >> "$PP_TEST_COMMAND_LOG"
case "$*" in
  *pp-lock-ready*) printf 'pp-lock-ready\n'; cat >/dev/null; exit 0 ;;
  'buildx inspect '*--bootstrap) exit 0 ;;
  'buildx inspect '*) exit 1 ;;
  *'image inspect --format '{{.Id}}*|*"image inspect --format '{{.Id}}'"*) printf 'sha256:%064d\n' 1; exit 0 ;;
  'info --format '*) echo /; exit 0 ;;
  *'{{.Size}}'*) echo 1024; exit 0 ;;
  'save -o '*) touch "$3" ;;
esac
if [ "$2" = 'sh -s' ]; then cat >/dev/null; fi
exit 0
`)
	}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	if err := d.Deploy(); err != nil {
		t.Fatalf("deploy: %v\n%s", err, out.String())
	}
	state, err := LoadState(root, project)
	if err != nil || state.ResourceID != "shop" || state.PreviousReleaseID != "current" {
		t.Fatalf("deploy lost adoption: %+v, %v", state, err)
	}
	deployed := state.CurrentReleaseID
	if err := d.Rollback(); err != nil {
		t.Fatalf("rollback: %v\n%s", err, out.String())
	}
	state, err = LoadState(root, project)
	if err != nil || state.ResourceID != "shop" || state.CurrentReleaseID != "current" || state.PreviousReleaseID != deployed {
		t.Fatalf("rollback lost adoption: %+v, %v", state, err)
	}
	commands := string(mustRead(t, log))
	if strings.Contains(commands, "-p 'shop_prod'") || strings.Count(commands, "-p 'shop'") != 2 {
		t.Fatalf("compose resource changed: %s", commands)
	}
}

func TestConfigDefaultsToV1WithoutRewriting(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=example\nSESSION_SECRET=example\nQUEUE_URL=example\n")
	path := filepath.Join(root, "deploy.yml")
	if err := writeYAML(path, validConfig()); err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(string(mustRead(t, path)), "version: 1\n")
	writeFile(t, root, "deploy.yml", body)
	cfg, err := LoadConfig(root, "deploy.yml")
	if err != nil || cfg.Version != 1 {
		t.Fatalf("missing version: %+v, %v", cfg, err)
	}
	if string(mustRead(t, path)) != body {
		t.Fatal("rewrote user config")
	}
	for _, version := range []string{"0", "-1", "99"} {
		writeFile(t, root, "deploy.yml", "version: "+version+"\n"+body)
		if _, err := LoadConfig(root, "deploy.yml"); err == nil || !strings.Contains(err.Error(), "unsupported config version") {
			t.Fatalf("accepted config version %s: %v", version, err)
		}
	}
}

func TestAdoptedHostOwnership(t *testing.T) {
	for _, test := range []struct {
		name, owner, labels, scoped string
		adopted, blocked            bool
	}{
		{name: "adopt matching containers", labels: "shop:prod", adopted: true},
		{name: "resume claim", labels: "shop:prod", owner: "1:shop:prod", adopted: true},
		{name: "wrong environment", labels: "shop:dev", adopted: true, blocked: true},
		{name: "another checkout claimed resources", owner: "1:shop:dev", adopted: true, blocked: true},
		{name: "future owner version", owner: "99:shop:prod", adopted: true, blocked: true},
		{name: "both layouts exist", labels: "shop:prod", scoped: "new", adopted: true, blocked: true},
		{name: "new dev beside adopted prod", owner: "1:shop:prod", labels: "shop:prod"},
		{name: "new dev cannot ignore unclaimed prod", labels: "shop:prod", blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeExecutable(t, root, "docker", `#!/bin/sh
case "$1" in
  ps) case "$*" in *project=shop_prod*) printf '%s' "$PP_TEST_SCOPED";; *) echo old;; esac ;;
  volume) echo shop_data ;;
  inspect) printf '%s' "$PP_TEST_LABELS" ;;
  *) exit 2 ;;
esac
`)
			ownerPath := filepath.Join(root, ".pp", "shop", "resource-owner")
			if test.owner != "" {
				writeNestedFile(t, root, ".pp/shop/resource-owner", test.owner+"\n")
			}
			project := Project{Name: "shop", Environment: "dev"}
			if test.adopted {
				project.Environment, project.resourceID = "prod", "shop"
			}
			cmd := exec.Command("sh", "-c", resourceHostCheck(project, test.adopted))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_TEST_LABELS="+test.labels, "PP_TEST_SCOPED="+test.scoped)
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.blocked {
				t.Fatalf("blocked=%t, err=%v, output=%s", test.blocked, err, out)
			}
			if test.adopted && !test.blocked && strings.TrimSpace(string(mustRead(t, ownerPath))) != "1:shop:prod" {
				t.Fatal("wrong resource claim")
			}
			if test.blocked && test.owner != "" && strings.TrimSpace(string(mustRead(t, ownerPath))) != test.owner {
				t.Fatal("overwrote another owner's claim")
			}
		})
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func writeNestedFile(t *testing.T, root, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, path, body)
}
