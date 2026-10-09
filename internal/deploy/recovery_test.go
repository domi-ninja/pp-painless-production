package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryUsesSavedInputs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".env.prod", "DATABASE_URL=old\nSESSION_SECRET=old\nQUEUE_URL=old\n")
	cfg := validConfig()
	cfg.Hooks = nil
	cfg.Checks = map[string][]Check{"smoke": {{Name: "saved", URL: "https://old.example/health"}}}
	plan := BuildPlan(cfg, GitMetadata{SHA: "abcdef1", ShortSHA: "abcdef1"}, time.Now())
	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	for _, image := range bundle.Images {
		writeNestedFile(t, root, mustRelative(t, root, image.Tar), "archive")
	}
	record := recordFromPlan(plan, bundle, "")
	if err := os.Remove(filepath.Join(root, ".env.prod")); err != nil {
		t.Fatal(err)
	}
	saved, err := recoveryPlan(root, record)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Config.Checks["smoke"][0].URL != "https://old.example/health" || saved.ReleaseID != record.ReleaseID {
		t.Fatalf("wrong recovery: %+v", saved)
	}
	values, err := loadSelectedEnv(root, saved.Config.Migrations.Env)
	if err != nil || values["DATABASE_URL"] != "old" {
		t.Fatalf("saved migration input: %v %v", values, err)
	}
	if err := os.Remove(bundle.Hosts[0].EnvFiles[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryPlan(root, record); err == nil || !strings.Contains(err.Error(), "recovery env") {
		t.Fatalf("missing env: %v", err)
	}
}

func TestRecoveryRefusesUnsavedScriptsAndLegacyMigration(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	record, err := LoadRelease(root, project, "previous")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryPlan(root, record); err != nil {
		t.Fatalf("simple legacy release: %v", err)
	}
	record.Migration.Status = "ok"
	if _, err := recoveryPlan(root, record); err == nil {
		t.Fatal("guessed legacy migration")
	}
	for _, contract := range []recoveryContract{{Version: 1, HasHooks: true}, {Version: 1, HasCommandChecks: true}, {Version: 99}} {
		if err := writeJSON(filepath.Join(record.BundlePath, "recovery.json"), contract); err != nil {
			t.Fatal(err)
		}
		if _, err := recoveryPlan(root, record); err == nil {
			t.Fatal("accepted unsafe recovery contract")
		}
	}
}
func mustRelative(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}
