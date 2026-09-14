package deploy

import (
	"reflect"
	"strings"
	"testing"
)

func TestPublishedPortsDefaultToLoopback(t *testing.T) {
	plan := Plan{}
	plan.SetAutoPort("host", "web", 80, 18001)
	for _, test := range []struct {
		name string
		port Port
		want string
	}{
		{"fixed default", Port{Published: FixedPort(8080), Target: 80}, "127.0.0.1:8080:80"},
		{"auto default", Port{Published: AutoPort(), Target: 80}, "127.0.0.1:18001:80"},
		{"fixed explicit public", Port{HostIP: "0.0.0.0", Published: FixedPort(8080), Target: 80}, "0.0.0.0:8080:80"},
		{"auto explicit public", Port{HostIP: "0.0.0.0", Published: AutoPort(), Target: 80}, "0.0.0.0:18001:80"},
		{"explicit private interface", Port{HostIP: "10.0.0.2", Published: FixedPort(8080), Target: 80}, "10.0.0.2:8080:80"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := renderPorts(plan, "host", "web", []Port{test.port})
			if !reflect.DeepEqual(got, []string{test.want}) {
				t.Fatalf("ports = %v, want [%s]", got, test.want)
			}
		})
	}
}

func TestAutoPortScriptUsesStableKeyAndRange(t *testing.T) {
	script := autoPortScript("quotes:web:80")
	for _, wanted := range []string{
		"/etc/pp/ports.tsv",
		"/etc/pp/ports.lock",
		"quotes:web:80",
		"seq 18000 19999",
	} {
		if !strings.Contains(script, wanted) {
			t.Fatalf("script missing %q:\n%s", wanted, script)
		}
	}
}
