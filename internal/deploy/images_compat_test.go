package deploy

import (
	"path/filepath"
	"testing"
)

func TestImagesForHostAcceptsSavedComposeVolumeAndEnvForms(t *testing.T) {
	for name, fields := range map[string]string{
		"long form from deployed canary": `    env_file:
      - path: env/web.env
        format: raw
    volumes:
      - type: volume
        source: canary-data
        target: /data
`,
		"short form from older bundles": `    env_file:
      - env/web.env
    volumes:
      - canary-data:/data
`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			host := HostBundle{Path: filepath.Join(root, "bundle", "hosts", "p3")}
			host.Compose = filepath.Join(host.Path, "compose.yml")
			writeNestedFile(t, root, "bundle/hosts/p3/compose.yml", "services:\n  web:\n    image: canary:recorded\n"+fields)
			images, err := imagesForHost(host, []ImageBundle{
				{ID: "web", Tags: []string{"canary:recorded"}},
				{ID: "unrelated", Tags: []string{"another:release"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(images) != 1 || images[0].ID != "web" {
				t.Fatalf("selected images: %+v", images)
			}
		})
	}
}
