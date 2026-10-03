package deploy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExamplesValidate(t *testing.T) {
	for _, name := range []string{"simple-website", "stateful-go-website", "convex-website"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join("..", "..", "examples", name)
			if err := os.CopyFS(root, os.DirFS(source)); err != nil {
				t.Fatal(err)
			}
			for _, pair := range [][2]string{{".env.example", ".env"}, {".env.local.example", ".env.local"}} {
				body, err := os.ReadFile(filepath.Join(root, pair[0]))
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, pair[1]), body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadConfig(root, "deploy.yml"); err != nil {
				t.Fatalf("example no longer matches the CLI: %v", err)
			}
		})
	}
}
