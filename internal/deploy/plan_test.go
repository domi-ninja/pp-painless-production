package deploy

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestBuildPlanUsesTimestampPlusSHAReleaseID(t *testing.T) {
	cfg := validConfig()
	git := GitMetadata{SHA: "abcdef123456", ShortSHA: "abcdef1", Dirty: true, DiffDigest: "digest"}
	now := time.Date(2026, 6, 28, 14, 22, 33, 0, time.UTC)

	plan := BuildPlan(cfg, git, now)
	if plan.ReleaseID != "20260628T142233Z-abcdef1" {
		t.Fatalf("unexpected release id %s", plan.ReleaseID)
	}

	var out bytes.Buffer
	PrintPlan(&out, plan)
	text := out.String()
	for _, wanted := range []string{
		"project: quotes",
		"release: 20260628T142233Z-abcdef1",
		"dirty: true",
		"worktree_diff_digest: digest",
		"  - app-01",
		"    services: web",
		"rollback: true",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("plan output missing %q:\n%s", wanted, text)
		}
	}
}

func TestPlanShowsOperationInputsWithoutSecretValues(t *testing.T) {
	cfg := validConfig()
	cfg.Build.Args = map[string]string{"PUBLIC_URL": "https://example.com"}
	web := cfg.Services["web"]
	web.Environment = map[string]string{"SECRET": "synthetic-secret"}
	web.Volumes = []VolumeMount{{Name: "data", Target: "/data"}}
	cfg.Services["web"] = web
	plan := BuildPlan(cfg, GitMetadata{SHA: "revision"}, time.Now())
	var out bytes.Buffer
	PrintPlan(&out, plan)
	for _, want := range []string{"services:", "DATABASE_URL", "public build arg keys: PUBLIC_URL", "mount: volume data -> /data", "recovery:", "quotes:revision"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "synthetic-secret") {
		t.Fatal("plan exposed inline secret")
	}
}
