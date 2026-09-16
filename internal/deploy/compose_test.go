package deploy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRenderBundleWritesComposeAndEnv(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")
	cfg := validConfig()
	cfg.Routes = []Route{{
		Host:    "quotes.example.com",
		Service: "web",
		Target:  "http://127.0.0.1:443",
	}}
	git := GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1", Dirty: true, DiffDigest: "digest"}
	plan := BuildPlan(cfg, git, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))

	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ImageTar != filepath.Join(root, ".deploy", "quotes", "prod", "releases", "20260628T100000Z-abcdef1", "images", "quotes-20260628T100000Z-abcdef1.tar") {
		t.Fatalf("unexpected image tar %s", bundle.ImageTar)
	}
	if len(bundle.Hosts) != 2 {
		t.Fatalf("expected 2 host bundles, got %d", len(bundle.Hosts))
	}

	compose, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	for _, wanted := range []string{
		"image: quotes:abcdef1234567890",
		"- 127.0.0.1:443:3000",
		"path: env/web.env",
		"format: raw",
		"pp.release: 20260628T100000Z-abcdef1",
	} {
		if !strings.Contains(composeText, wanted) {
			t.Fatalf("compose missing %q:\n%s", wanted, composeText)
		}
	}

	env, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "env", "web.env"))
	if err != nil {
		t.Fatal(err)
	}
	envText := string(env)
	if !strings.Contains(envText, "DATABASE_URL=postgres://example\n") || !strings.Contains(envText, "SESSION_SECRET=secret\n") {
		t.Fatalf("unexpected env file:\n%s", envText)
	}

	routes, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "routes.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	routeText := string(routes)
	if !strings.Contains(routeText, "quotes.example.com {\n\treverse_proxy http://127.0.0.1:443\n}") {
		t.Fatalf("unexpected route file:\n%s", routeText)
	}
}

func TestRenderBundleSupportsHostIPPortBinding(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")
	cfg := validConfig()
	web := cfg.Services["web"]
	web.Ports = []Port{{HostIP: "127.0.0.1", Published: FixedPort(8081), Target: 80}}
	cfg.Services["web"] = web
	git := GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}
	plan := BuildPlan(cfg, git, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))

	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "- 127.0.0.1:8081:80") {
		t.Fatalf("compose missing host ip port binding:\n%s", string(compose))
	}
}

func TestRenderBundleUsesAllocatedAutoPortForComposeAndRoute(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")
	cfg := validConfig()
	web := cfg.Services["web"]
	web.Ports = []Port{{Published: AutoPort(), Target: 80}}
	cfg.Services["web"] = web
	cfg.Routes = []Route{{
		Host:    "quotes.example.com",
		Service: "web",
	}}
	git := GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}
	plan := BuildPlan(cfg, git, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	plan.SetAutoPort("app-01", "web", 80, 18001)

	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(compose), "- 127.0.0.1:18001:80") {
		t.Fatalf("compose missing allocated auto port:\n%s", string(compose))
	}
	routes, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "app-01", "routes.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(routes), "reverse_proxy http://127.0.0.1:18001") {
		t.Fatalf("route missing allocated auto port:\n%s", string(routes))
	}
}

func TestRenderBundleSupportsConvexStyleExtensions(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.local", "VITE_SITE_URL=https://humanist.design\nVITE_CONVEX_URL=https://api.humanist.design\n")
	writeFile(t, root, ".env.backend", "POSTGRES_URL=postgres://humanist:secret@postgres:5432/humanist\n")
	writeFile(t, root, "humanist.caddy.tmpl", "api.humanist.design {\n\treverse_proxy 127.0.0.1:${service.convex-backend.port.3210}\n}\n")
	cfg := Config{
		Version: 1,
		Project: Project{
			Name:        "humanist-design",
			Environment: "prod",
		},
		Env: EnvSpec{Source: ".env.local"},
		Builds: map[string]Build{
			"web": {
				Context:    ".",
				Dockerfile: "Dockerfile",
				Tags:       []string{"humanist-design-web:${release}"},
				Args: map[string]string{
					"VITE_SITE_URL": "${env.VITE_SITE_URL}",
				},
			},
		},
		Hosts: map[string]Host{
			"p3": {SSH: "deploy@p3.domi.ninja"},
		},
		Services: map[string]Service{
			"web": {
				Image:  "humanist-design-web:${release}",
				Build:  "web",
				Phase:  "frontend",
				Hosts:  []string{"p3"},
				Ports:  []Port{{Published: AutoPort(), Target: 80}},
				Health: Health{HTTP: "https://humanist.design/"},
			},
			"convex-backend": {
				Image:  "ghcr.io/get-convex/convex-backend:latest",
				Pull:   "if_missing",
				Phase:  "backend",
				Hosts:  []string{"p3"},
				Env:    EnvSpec{Source: ".env.backend", Required: []string{"POSTGRES_URL"}},
				Ports:  []Port{{Published: AutoPort(), Target: 3210}},
				Health: Health{Command: []string{"curl", "-sf", "http://127.0.0.1:3210/version"}},
			},
			"s3": {
				Image:       "minio/minio:latest",
				Pull:        "if_missing",
				Phase:       "infra",
				Hosts:       []string{"p3"},
				Command:     []string{"server", "/data"},
				Environment: map[string]string{"MINIO_BROWSER_REDIRECT_URL": "${env.VITE_SITE_URL}"},
				Volumes:     []VolumeMount{{Source: "/data/pp/humanist-design/s3", Target: "/data", Type: "bind"}},
				Health:      Health{Command: []string{"curl", "-sf", "http://127.0.0.1:9000/minio/health/ready"}},
			},
		},
		RouteFiles: []RouteFile{{
			Source: "humanist.caddy.tmpl",
			Host:   "p3",
		}},
		Hooks: map[string][]Hook{
			"after_backend_healthy": {{
				Name: "sync convex",
				Run:  []string{"npx", "convex", "dev", "--once", "--env-file", ".env.local"},
				Env:  EnvSpec{Source: ".env.local", Required: []string{"VITE_SITE_URL"}},
			}},
		},
	}
	if err := ValidateConfig(root, cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
	git := GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}
	plan := BuildPlan(cfg, git, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	plan.SetAutoPort("p3", "web", 80, 18001)
	plan.SetAutoPort("p3", "convex-backend", 3210, 18002)

	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Images) != 1 {
		t.Fatalf("expected one local image, got %d", len(bundle.Images))
	}
	if bundle.Images[0].ID != "web" || bundle.Images[0].Tags[0] != "humanist-design-web:20260628T100000Z-abcdef1" {
		t.Fatalf("unexpected image bundle: %#v", bundle.Images[0])
	}
	host := bundle.Hosts[0]
	if strings.Join(host.PullServices, ",") != "convex-backend,s3" {
		t.Fatalf("unexpected pull services: %#v", host.PullServices)
	}
	compose, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "p3", "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	composeText := string(compose)
	for _, wanted := range []string{
		"image: humanist-design-web:20260628T100000Z-abcdef1",
		"- 127.0.0.1:18001:80",
		"- 127.0.0.1:18002:3210",
		"- /data/pp/humanist-design/s3:/data",
		"MINIO_BROWSER_REDIRECT_URL: https://humanist.design",
	} {
		if !strings.Contains(composeText, wanted) {
			t.Fatalf("compose missing %q:\n%s", wanted, composeText)
		}
	}
	routes, err := os.ReadFile(filepath.Join(bundle.Root, "hosts", "p3", "routes.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(routes), "reverse_proxy 127.0.0.1:18002") {
		t.Fatalf("route template was not rendered:\n%s", string(routes))
	}
}

func TestReleaseRecordPreservesMultipleImageBundles(t *testing.T) {
	plan := BuildPlan(validConfig(), GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	bundle := Bundle{
		Root:      "/tmp/release",
		ImageTar:  "/tmp/release/images/web.tar",
		ImageTags: []string{"web:release"},
		Images: []ImageBundle{
			{ID: "web", Tar: "/tmp/release/images/web.tar", Tags: []string{"web:release"}},
			{ID: "worker", Tar: "/tmp/release/images/worker.tar", Tags: []string{"worker:release"}},
		},
		Hosts: []HostBundle{{
			ID:         "app-01",
			SSH:        "deploy@app-01.example.com",
			RemoteDir:  ".pp/quotes/releases/release",
			ServiceIDs: []string{"web"},
		}},
	}

	record := recordFromPlan(plan, bundle, "")
	if len(record.Images) != 2 {
		t.Fatalf("expected two image records, got %#v", record.Images)
	}
	roundTrip := bundleFromRecord(record)
	if len(roundTrip.Images) != 2 || roundTrip.Images[1].ID != "worker" {
		t.Fatalf("expected image bundles to round trip, got %#v", roundTrip.Images)
	}
}

func TestRunChecksValidatesExpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	plan := BuildPlan(validConfig(), GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	plan.Config.Checks = map[string][]Check{
		"smoke": {{
			Name:         "empty",
			URL:          server.URL,
			ExpectStatus: http.StatusNoContent,
		}},
	}
	var out bytes.Buffer
	deployer := Deployer{Root: t.TempDir(), Out: &out, Err: &out}
	records, err := deployer.RunChecks(plan, "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != "ok" || records[0].ActualStatus != http.StatusNoContent {
		t.Fatalf("unexpected check records: %#v", records)
	}
	if !strings.Contains(out.String(), "check empty:") {
		t.Fatalf("expected check output, got %q", out.String())
	}
}

func TestRunChecksCanValidateRedirectWithoutFollowing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/target")
		w.WriteHeader(http.StatusPermanentRedirect)
	}))
	defer server.Close()

	followRedirects := false
	plan := BuildPlan(validConfig(), GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	plan.Config.Checks = map[string][]Check{
		"smoke": {{
			Name:            "www redirect",
			URL:             server.URL,
			ExpectStatus:    http.StatusPermanentRedirect,
			FollowRedirects: &followRedirects,
		}},
	}
	var out bytes.Buffer
	deployer := Deployer{Root: t.TempDir(), Out: &out, Err: &out}
	records, err := deployer.RunChecks(plan, "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ActualStatus != http.StatusPermanentRedirect || records[0].FollowRedirects {
		t.Fatalf("unexpected redirect record: %#v", records)
	}
}

func TestRunChecksSupportsCommandChecksWithEnv(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "SMOKE_VALUE=ok\n")
	plan := BuildPlan(validConfig(), GitMetadata{SHA: "abcdef1234567890", ShortSHA: "abcdef1"}, time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC))
	plan.Config.Checks = map[string][]Check{
		"smoke": {{
			Name:    "command",
			Command: []string{"/bin/sh", "-c", "test \"$SMOKE_VALUE\" = ok"},
			Env:     EnvSpec{Source: ".env.prod", Required: []string{"SMOKE_VALUE"}},
		}},
	}
	var out bytes.Buffer
	deployer := Deployer{Root: root, Out: &out, Err: &out}
	records, err := deployer.RunChecks(plan, "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Type != "command" || records[0].EnvSource != ".env.prod" || records[0].Status != "ok" {
		t.Fatalf("unexpected command check records: %#v", records)
	}
}

func TestDownProjectContainersCommandUsesProjectAndEnvironmentLabels(t *testing.T) {
	command := downProjectContainersCommand(Project{Name: "humanist-design", Environment: "dev"})
	for _, wanted := range []string{
		"docker ps -aq --filter 'label=pp.project=humanist-design'",
		"docker rm -f $ids",
		"no containers for deployment humanist-design_dev",
		"--filter 'label=pp.environment=dev'",
	} {
		if !strings.Contains(command, wanted) {
			t.Fatalf("down command missing %q: %s", wanted, command)
		}
	}
	if strings.Contains(command, "docker volume") || strings.Contains(command, "docker system") {
		t.Fatalf("down command should not remove data: %s", command)
	}
}
