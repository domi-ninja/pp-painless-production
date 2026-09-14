package deploy

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeploymentIdentitySeparatesAmbiguousNames(t *testing.T) {
	a := Project{Name: "shop-dev", Environment: "prod"}
	b := Project{Name: "shop", Environment: "dev-prod"}
	if a.DeploymentID() == b.DeploymentID() {
		t.Fatal("deployment identities collide")
	}
	if err := (Project{Name: "shop_dev", Environment: "prod"}).validate(); err == nil {
		t.Fatal("underscore must not be accepted in identity components")
	}
}

func TestEnvironmentBundlesAndStateAreIndependent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=example\nSESSION_SECRET=example\nQUEUE_URL=example\n")
	for _, environment := range []string{"dev", "prod"} {
		cfg := validConfig()
		cfg.Project.Environment = environment
		plan := BuildPlan(cfg, GitMetadata{SHA: "abcdef123", ShortSHA: "abcdef1"}, time.Unix(100, 0))
		bundle, err := RenderBundle(root, plan)
		if err != nil {
			t.Fatal(err)
		}
		wantDir := filepath.Join(root, ".deploy", "quotes", environment, "releases", plan.ReleaseID)
		if bundle.Root != wantDir || bundle.Hosts[0].RemoteDir != ".pp/quotes/"+environment+"/releases/"+plan.ReleaseID {
			t.Fatalf("wrong bundle paths: %#v", bundle)
		}
		record := recordFromPlan(plan, bundle, "")
		record.Status = environment
		if err := SaveRelease(root, record); err != nil {
			t.Fatal(err)
		}
		if err := SaveState(root, State{Project: "quotes", Environment: environment, CurrentReleaseID: plan.ReleaseID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, environment := range []string{"dev", "prod"} {
		project := Project{Name: "quotes", Environment: environment}
		state, err := LoadState(root, project)
		if err != nil {
			t.Fatal(err)
		}
		record, err := LoadRelease(root, project, state.CurrentReleaseID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status != environment {
			t.Fatalf("loaded another environment: %#v", record)
		}
	}
}

func TestReleaseRejectsWrongOwnerAndPaths(t *testing.T) {
	root := t.TempDir()
	project := Project{Name: "shop", Environment: "dev"}
	for _, test := range []struct {
		name   string
		change func(*ReleaseRecord)
	}{
		{"environment", func(r *ReleaseRecord) { r.Environment = "prod" }},
		{"project", func(r *ReleaseRecord) { r.Project = "other" }},
		{"release", func(r *ReleaseRecord) { r.ReleaseID = "other" }},
		{"bundle", func(r *ReleaseRecord) {
			r.BundlePath = filepath.Join(root, ".deploy", "shop", "prod", "releases", "release")
		}},
		{"remote", func(r *ReleaseRecord) { r.Hosts[0].RemoteDir = ".pp/shop/prod/releases/release" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			record := ReleaseRecord{Project: "shop", Environment: "dev", ReleaseID: "release",
				BundlePath: filepath.Join(project.localDir(root), "releases", "release"),
				Hosts:      []HostRecord{{RemoteDir: remoteReleaseDir(project, "release")}},
			}
			path := releaseMetadataPath(root, project, "release")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			test.change(&record)
			if err := writeJSON(path, record); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadRelease(root, project, "release"); err == nil {
				t.Fatal("accepted mismatched record")
			}
		})
	}
	if _, err := LoadRelease(root, project, "../../prod/release"); err == nil {
		t.Fatal("accepted traversal")
	}
}

func TestStateRejectsWrongOwner(t *testing.T) {
	root := t.TempDir()
	project := Project{Name: "shop", Environment: "dev"}
	if err := SaveState(root, State{Project: "shop", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(statePath(root, project), State{Project: "shop", Environment: "prod"}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(root, project); err == nil || !strings.Contains(err.Error(), "ownership mismatch") {
		t.Fatalf("got %v", err)
	}
}

func TestRemoteCommandsUseSelectedEnvironment(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "commands")
	t.Setenv("PP_TEST_COMMAND_LOG", log)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, root, "ssh", "#!/bin/sh\nprintf '%s\\n' \"$2\" >> \"$PP_TEST_COMMAND_LOG\"\ncase \"$2\" in *ports.tsv*) echo 18001;; esac\n")
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	for _, project := range []Project{
		{Name: "shop", Environment: "dev"},
		{Name: "shop", Environment: "prod"},
		{Name: "shop", Environment: "prod", resourceID: "shop"},
	} {
		plan := Plan{Config: Config{Project: project, Services: map[string]Service{"web": {Ports: []Port{{Published: AutoPort(), Target: 80}}}}}, Hosts: []HostPlan{{ID: "host", SSH: "unused", Services: []string{"web"}}}}
		if err := d.ResolveAutoPorts(&plan); err != nil {
			t.Fatal(err)
		}
		host := HostBundle{ID: "host", SSH: "unused", RemoteDir: remoteReleaseDir(project, "release"), Routes: "routes.caddy"}
		if err := d.ComposeUp(plan, host, nil, false); err != nil {
			t.Fatal(err)
		}
		if err := d.ApplyRoutes(plan, Bundle{Hosts: []HostBundle{host}}); err != nil {
			t.Fatal(err)
		}
		if err := d.Remote(host.SSH, downProjectContainersCommand(project)); err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"key='shop_dev:web:80'", "key='shop_prod:web:80'", "key='shop:web:80'", "-p 'shop_dev'", "-p 'shop_prod'", "-p 'shop'", "up -d --pull never", "/routes/shop_dev.caddy", "/routes/shop_prod.caddy", "/routes/shop.caddy", "label=pp.environment=dev", "label=pp.environment=prod"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("missing %q in commands", want)
		}
	}
}

func TestLegacyHostPreflight(t *testing.T) {
	for _, test := range []struct {
		name, containers, volumes          string
		acknowledged, dockerFails, blocked bool
	}{
		{name: "new host"},
		{name: "legacy container", containers: "old-web", blocked: true},
		{name: "legacy volume", volumes: "old-data", blocked: true},
		{name: "explicitly migrated volume", volumes: "old-data", acknowledged: true},
		{name: "ack cannot bypass old container", containers: "old-web", acknowledged: true, blocked: true},
		{name: "docker unavailable", dockerFails: true, blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeExecutable(t, root, "docker", "#!/bin/sh\n[ \"$PP_TEST_DOCKER_FAILS\" = true ] && exit 1\ncase \"$1\" in ps) printf '%s' \"$PP_TEST_CONTAINERS\";; volume) printf '%s' \"$PP_TEST_VOLUMES\";; *) exit 2;; esac\n")
			if test.acknowledged {
				if err := os.MkdirAll(filepath.Join(root, ".pp", "shop"), 0755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, root, ".pp/shop/.environment-isolation-migrated", "")
			}
			cmd := exec.Command("sh", "-c", legacyHostCheck(Project{Name: "shop", Environment: "dev"}))
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_TEST_CONTAINERS="+test.containers, "PP_TEST_VOLUMES="+test.volumes)
			if test.dockerFails {
				cmd.Env = append(cmd.Env, "PP_TEST_DOCKER_FAILS=true")
			}
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.blocked {
				t.Fatalf("blocked=%t, err=%v, output=%s", test.blocked, err, out)
			}
		})
	}
}

func TestRollbackRejectsOtherEnvironmentBeforeRunningCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=example\nSESSION_SECRET=example\nQUEUE_URL=example\n")
	cfg := validConfig()
	cfg.Project.Environment = "dev"
	if err := writeYAML(filepath.Join(root, "deploy.dev.yml"), cfg); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "deploy.dev.yml"}, {"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	project := cfg.Project
	for _, id := range []string{"current", "previous"} {
		record := ReleaseRecord{Project: project.Name, Environment: "dev", ReleaseID: id, BundlePath: filepath.Join(project.localDir(root), "releases", id)}
		if err := SaveRelease(root, record); err != nil {
			t.Fatal(err)
		}
		if id == "previous" {
			record.Environment = "prod"
			if err := writeJSON(releaseMetadataPath(root, project, id), record); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := SaveState(root, State{Project: project.Name, Environment: "dev", CurrentReleaseID: "current", PreviousReleaseID: "previous"}); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "unexpected-command")
	t.Setenv("PP_TEST_COMMAND_LOG", log)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, name := range []string{"ssh", "docker"} {
		writeExecutable(t, root, name, "#!/bin/sh\necho called >> \"$PP_TEST_COMMAND_LOG\"\nexit 1\n")
	}
	var out bytes.Buffer
	d := NewDeployer(root, &out, &out)
	d.ConfigPath = "deploy.dev.yml"
	if err := d.Rollback(); err == nil || !strings.Contains(err.Error(), "ownership mismatch") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("rollback ran an external command: %v", err)
	}
	// An unsupported state format must stop before builds or host changes.
	writeFile(t, root, filepath.Join(".deploy", project.Name, project.Environment, "state.json"), `{"version":99,"project":"quotes","environment":"dev"}`)
	if err := d.Deploy(); err == nil || !strings.Contains(err.Error(), "unsupported state version") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("deployment ran an external command: %v", err)
	}
}

func writeExecutable(t *testing.T, root, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
}
