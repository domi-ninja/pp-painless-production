package deploy

import (
	"gopkg.in/yaml.v3"
	"io"
	"strings"
	"testing"
	"time"
)

func TestVolumeTypesSurviveComposeRendering(t *testing.T) {
	mounts := renderVolumeMounts([]VolumeMount{{Source: "/srv/data:old", Target: "/data", Type: "bind", ReadOnly: true}, {Name: "data", Target: "/cache"}})
	body, err := yaml.Marshal(ComposeFile{Services: map[string]ComposeService{"app": {Image: "image", Volumes: mounts}}})
	if err != nil {
		t.Fatal(err)
	}
	var compose ComposeFile
	if err := yaml.Unmarshal(body, &compose); err != nil {
		t.Fatal(err)
	}
	bind := compose.Services["app"].Volumes[0]
	volume := compose.Services["app"].Volumes[1]
	if bind.Type != "bind" || bind.Source != "/srv/data:old" || bind.Bind == nil || bind.Bind.CreateHostPath || !bind.ReadOnly || volume.Type != "volume" {
		t.Fatalf("lost mount contract: %s", body)
	}
}

func TestUpstreamOnlyDeployNeedsNoBuilder(t *testing.T) {
	root := t.TempDir()
	cfg := Config{Version: 1, Project: Project{Name: "shop", Environment: "prod"}, Hosts: map[string]Host{"app": {SSH: "unused"}}, Services: map[string]Service{"cache": {Image: "redis:7", Pull: "if_missing", Hosts: []string{"app"}}}}
	if err := ValidateConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	plan := BuildPlan(cfg, GitMetadata{}, time.Now())
	bundle, err := RenderBundle(root, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Images) != 0 {
		t.Fatal("rendered an unused build")
	}
	if err := NewDeployer(root, io.Discard, io.Discard).BuildImages(plan, bundle); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltServiceMustUseItsBuildTag(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Dockerfile", "FROM scratch\n")
	writeFile(t, root, ".env.prod", "DATABASE_URL=db\nSESSION_SECRET=secret\nQUEUE_URL=queue\n")
	cfg := validConfig()
	cfg.Build.Tags = []string{"wrong:${release}"}
	if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), "must match a tag") {
		t.Fatalf("mismatched legacy build: %v", err)
	}
	cfg.Builds = map[string]Build{"web": {Context: ".", Dockerfile: "Dockerfile", Tags: []string{"web:${release}"}}}
	svc := cfg.Services["web"]
	svc.Build = "web"
	svc.Image = "different:${release}"
	cfg.Services["web"] = svc
	if err := ValidateConfig(root, cfg); err == nil || !strings.Contains(err.Error(), "build web") {
		t.Fatalf("mismatched named build: %v", err)
	}
	svc.Image = "web:${release}"
	cfg.Services["web"] = svc
	if err := ValidateConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
}
