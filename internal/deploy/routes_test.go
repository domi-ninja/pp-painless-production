package deploy

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteTransactionKeepsOtherRoutesAndRestoresFailures(t *testing.T) {
	for _, failure := range []string{"", "validate", "reload"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			routes := filepath.Join(root, "proxy/routes")
			config := filepath.Join(root, "Caddyfile")
			source := filepath.Join(root, "new.caddy")
			writeNestedFile(t, root, "proxy/routes/other.caddy", "other.test { respond other }\n")
			writeNestedFile(t, root, "proxy/routes/shop_prod.caddy", "old.test { respond old }\n")
			writeFile(t, root, "Caddyfile", "import "+routes+"/*.caddy\n")
			writeFile(t, root, "new.caddy", "new.test { respond new }\n")
			writeExecutable(t, root, "caddy", `#!/bin/sh
if [ "$1" = validate ] && [ "$PP_ROUTE_FAIL" = validate ]; then exit 1; fi
if [ "$1" = reload ] && [ "$PP_ROUTE_FAIL" = reload ] && grep -q new.test "$PP_ROUTE_ACTIVE"; then exit 1; fi
`)
			cmd := exec.Command("sh", "-c", routeUpdateScript("shop_prod", source, routes, config))
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_ROUTE_FAIL="+failure, "PP_ROUTE_ACTIVE="+routes+"/shop_prod.caddy")
			out, err := cmd.CombinedOutput()
			if (err != nil) != (failure != "") {
				t.Fatalf("transaction: %v %s", err, out)
			}
			want := "new.test"
			if failure != "" {
				want = "old.test"
			}
			if !strings.Contains(string(mustRead(t, routes+"/shop_prod.caddy")), want) || !strings.Contains(string(mustRead(t, routes+"/other.caddy")), "other.test") {
				t.Fatal("changed the wrong route files")
			}
			if failure == "" {
				cmd = exec.Command("sh", "-c", routeUpdateScript("shop_prod", "", routes, config))
				cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("remove routes: %v %s", err, out)
				}
				if _, err := os.Stat(routes + "/shop_prod.caddy"); !os.IsNotExist(err) {
					t.Fatal("obsolete route remains")
				}
			}
		})
	}
}

func TestRemovedHostRequiresDeliberateShutdown(t *testing.T) {
	root := t.TempDir()
	project := legacyFixture(t, root)
	state, err := LoadState(root, project)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutable(t, root, "ssh", "#!/bin/sh\necho owned-container\n")
	d := NewDeployer(root, io.Discard, io.Discard)
	plan := Plan{Config: Config{Project: project}}
	if err := d.checkRemovedHosts(plan, state); err == nil || !strings.Contains(err.Error(), "still has owned containers") {
		t.Fatalf("removed running host: %v", err)
	}
	writeExecutable(t, root, "ssh", "#!/bin/sh\nif [ \"$2\" = 'sh -s' ]; then cat >/dev/null; fi\n")
	if err := d.checkRemovedHosts(plan, state); err != nil {
		t.Fatalf("stopped host: %v", err)
	}
}
