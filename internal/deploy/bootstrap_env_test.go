package deploy

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBootstrapPreservesQuotedIdentifiers(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "..", "examples", "convex-website"))); err != nil {
		t.Fatal(err)
	}
	// JWT generation is independent of identifier preservation.
	writeFile(t, root, "scripts/generate-jwt.js", "console.log('JWT_PUBLIC_KEY=test');\n")
	writeFile(t, root, ".env.local", "POSTGRES_USER=\"app@team\"\nPOSTGRES_DB='existing_db'\nINSTANCE_NAME=\"existing-instance\"\n")
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "bootstrap-prod-env.sh"), "example.test")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v %s", err, out)
	}
	values, err := LoadEnvFile(filepath.Join(root, ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	if values["POSTGRES_USER"] != "app@team" || values["POSTGRES_DB"] != "existing_db" || values["INSTANCE_NAME"] != "existing-instance" {
		t.Fatal("quoted data identifiers changed")
	}
	u, err := url.Parse(values["POSTGRES_URL"])
	if err != nil {
		t.Fatal(err)
	}
	if u.User.Username() != values["POSTGRES_USER"] {
		t.Fatal("URL username differs from PostgreSQL username")
	}
	if password, _ := u.User.Password(); password != values["POSTGRES_PASSWORD"] {
		t.Fatal("URL password differs")
	}
}
