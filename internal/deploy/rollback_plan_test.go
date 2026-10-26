package deploy

import "testing"

func TestRecordedReleaseDoesNotRunCurrentCheckoutHooksOrChecks(t *testing.T) {
	base := Plan{Config: Config{
		Hooks:    map[string][]Hook{"after_backend_healthy": {{Name: "new functions", Run: []string{"sh", "new-functions.sh"}}}},
		Checks:   map[string][]Check{"smoke": {{Name: "new route", URL: "https://example.test/new"}}},
		Services: map[string]Service{"backend": {Phase: "backend"}},
	}}
	record := ReleaseRecord{ReleaseID: "old", ResourceID: "app_prod", Git: GitMetadata{SHA: "old-sha"}}
	got := planForRecord(base, record)
	if len(got.Config.Hooks) != 0 || len(got.Config.Checks) != 0 {
		t.Fatal("rollback kept current checkout actions")
	}
	if !needsPhasedApply(got) {
		t.Fatal("lost phased service apply")
	}
	if got.ReleaseID != "old" || got.Git.SHA != "old-sha" {
		t.Fatal("lost recorded release identity")
	}
	if len(base.Config.Hooks) != 1 || len(base.Config.Checks) != 1 {
		t.Fatal("mutated current deploy plan")
	}
}
