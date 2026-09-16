package deploy

import (
	"strings"
	"testing"
)

func TestServiceEnvOnlyIncludesDeclaredKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "source.env", "DB_PASSWORD=db\nMINIO_PASSWORD=minio\nINSTANCE_SECRET=instance\n")
	for _, test := range []struct {
		name string
		env  EnvSpec
		want string
		fail bool
	}{
		{"database", EnvSpec{Source: "source.env", Required: []string{"DB_PASSWORD"}}, "DB_PASSWORD=db\n", false},
		{"storage", EnvSpec{Source: "source.env", Required: []string{"MINIO_PASSWORD"}}, "MINIO_PASSWORD=minio\n", false},
		{"explicit all", EnvSpec{Source: "source.env", IncludeAll: true}, "DB_PASSWORD=db\nINSTANCE_SECRET=instance\nMINIO_PASSWORD=minio\n", false},
		{"unspecified", EnvSpec{Source: "source.env"}, "", true},
		{"missing", EnvSpec{Source: "source.env", Required: []string{"MISSING"}}, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, err := renderServiceEnv(root, root, "service", test.env)
			if (err != nil) != test.fail {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && string(mustRead(t, path)) != test.want {
				t.Fatalf("unexpected env: %s", mustRead(t, path))
			}
		})
	}
	writeFile(t, root, ".env.prod", "DATABASE_URL=test\nSESSION_SECRET=test\nQUEUE_URL=test\n")
	cfg := validConfig()
	web := cfg.Services["web"]
	web.Env.Required = nil
	cfg.Services["web"] = web
	if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), "must list injected keys") {
		t.Fatalf("got %v", err)
	}
}
