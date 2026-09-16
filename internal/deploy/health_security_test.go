package deploy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHTTPHealthCheckCannotExecuteURL(t *testing.T) {
	if _, err := exec.LookPath("wget"); err != nil {
		t.Skip("execution test requires wget")
	}
	root := t.TempDir()
	marker := filepath.Join(root, "injected")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer server.Close()
	payload := server.URL + "/?probe=;touch${IFS}" + marker
	// Positive control: this exact input exploited the old CMD-SHELL renderer.
	if out, err := exec.Command("sh", "-c", "wget -q --spider "+payload).CombinedOutput(); err != nil {
		t.Fatalf("control: %v %s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("payload did not exploit the old renderer: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	service := Service{Health: Health{HTTP: "${env.HEALTH_URL}"}}
	rendered, err := renderComposeService(Plan{}, "host", "web", service, RenderVars{}, map[string]string{"HEALTH_URL": payload}, "")
	if err != nil {
		t.Fatal(err)
	}
	args := rendered.Healthcheck.Test
	if args[0] != "CMD" || args[len(args)-1] != payload {
		t.Fatalf("not a literal exec check: %q", args)
	}
	if out, err := exec.Command(args[1], args[2:]...).CombinedOutput(); err != nil {
		t.Fatalf("exec health check: %v %s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("URL executed a shell command")
	}
}

func TestHTTPHealthCheckValidatesRenderedURL(t *testing.T) {
	for _, value := range []string{"-O/tmp/file", "file:///etc/passwd", "http://", "http://user:pass@example.com/", "http://example.com/; touch /tmp/file", "http://example.com/\ncommand"} {
		_, err := renderComposeService(Plan{}, "host", "web", Service{Health: Health{HTTP: "${env.URL}"}}, RenderVars{}, map[string]string{"URL": value}, "")
		if err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}
