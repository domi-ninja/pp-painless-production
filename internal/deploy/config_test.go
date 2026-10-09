package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateConfigAcceptsValidConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")

	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	cfg := validConfig()
	if err := ValidateConfig(root, cfg); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateConfigRejectsUnknownHostAndPortConflict(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")

	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	cfg := validConfig()
	web := cfg.Services["web"]
	web.Hosts = []string{"app-01", "missing"}
	cfg.Services["web"] = web
	worker := cfg.Services["worker"]
	worker.Hosts = []string{"app-01"}
	worker.Ports = []Port{{Published: FixedPort(443), Target: 3001}}
	cfg.Services["worker"] = worker

	err := ValidateConfig(root, cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown host missing") {
		t.Fatalf("expected unknown host error, got %s", msg)
	}
	if !strings.Contains(msg, "published port 443 conflicts") {
		t.Fatalf("expected port conflict error, got %s", msg)
	}
}

func TestValidateConfigRequiresMigrationRollback(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")

	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	cfg := validConfig()
	cfg.Migrations.RollbackCommand = nil
	cfg.Migrations.RestorePlan = ""

	err := ValidateConfig(root, cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "migrations.rollback_command or migrations.restore_plan is required") {
		t.Fatalf("expected rollback requirement, got %s", err.Error())
	}
}

func TestValidateConfigRejectsInvalidRoute(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=postgres://example\nSESSION_SECRET=secret\nQUEUE_URL=redis://example\n")

	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	cfg := validConfig()
	cfg.Routes = []Route{{
		Host:    "quotes.example.com",
		Service: "missing",
		Target:  "tcp://127.0.0.1:8080",
	}}

	err := ValidateConfig(root, cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "routes[0].service references unknown service missing") {
		t.Fatalf("expected route service error, got %s", msg)
	}
	if !strings.Contains(msg, "routes[0].target must use http or https") {
		t.Fatalf("expected route target error, got %s", msg)
	}
}

func TestLoadEnvFileTrimsQuotes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env", "A='one'\nB=\"two\"\n")

	values, err := LoadEnvFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if values["A"] != "one" || values["B"] != "two" {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func validConfig() Config {
	return Config{
		Version: 1,
		Project: Project{
			Name:        "quotes",
			Environment: "prod",
		},
		Build: Build{
			Context:    ".",
			Dockerfile: "Dockerfile",
		},
		Hosts: map[string]Host{
			"app-01": {
				SSH: "deploy@app-01.example.com",
			},
			"worker-01": {
				SSH: "deploy@worker-01.example.com",
			},
		},
		Services: map[string]Service{
			"web": {
				Image: "quotes:${git_sha}",
				Hosts: []string{"app-01"},
				Env: EnvSpec{
					Source:   ".env.prod",
					Required: []string{"DATABASE_URL", "SESSION_SECRET"},
				},
				Ports:  []Port{{Published: FixedPort(443), Target: 3000}},
				Health: Health{HTTP: "https://quotes.example.com/health"},
			},
			"worker": {
				Image: "quotes:${git_sha}",
				Hosts: []string{"worker-01"},
				Env: EnvSpec{
					Source:   ".env.prod",
					Required: []string{"DATABASE_URL", "QUEUE_URL"},
				},
				Health: Health{Command: []string{"node", "scripts/healthcheck-worker.js"}},
			},
		},
		Migrations: &Migration{
			Image:           "quotes:${git_sha}",
			Command:         []string{"pnpm", "db:migrate:prod"},
			RollbackCommand: []string{"pnpm", "db:rollback:prod"},
			Env: EnvSpec{
				Source:   ".env.prod",
				Required: []string{"DATABASE_URL"},
			},
			Run: "before_services",
		},
	}
}

func writeFile(t *testing.T, root string, name string, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
