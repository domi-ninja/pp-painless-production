package deploy

import (
	"path/filepath"
	"testing"
)

func TestTemplatesRenderArgumentsAndFixedPortsWithoutExpandingEnvValues(t *testing.T) {
	plan := Plan{Config: Config{Project: Project{Name: "shop", Environment: "prod"}, Services: map[string]Service{"web": {Hosts: []string{"app"}, Ports: []Port{{Published: FixedPort(8080), Target: 80}}}}}, Hosts: []HostPlan{{ID: "app"}}, ReleaseID: "release"}
	values := map[string]string{"VALUE": "literal${service.web.port.80}"}
	service, err := renderComposeService(plan, "app", "web", Service{Command: []string{"echo", "${release}", "${service.web.port.80}", "${env.VALUE}"}, Entrypoint: []string{"/bin/${project}"}}, RenderVars{}, values, "")
	if err != nil {
		t.Fatal(err)
	}
	if service.Command[1] != "release" || service.Command[2] != "8080" || service.Command[3] != values["VALUE"] || service.Entrypoint[0] != "/bin/shop" {
		t.Fatalf("arguments changed: %+v", service)
	}
	if err := validateTemplateInput("${env.MISSING}", plan, "app", values); err == nil {
		t.Fatal("accepted missing env placeholder")
	}
	if err := validateTemplateInput("${service.unknown.port.80}", plan, "app", values); err == nil {
		t.Fatal("accepted unknown service placeholder")
	}
}

func TestEnvGrammarRejectsDuplicatesBadKeysAndUnmatchedQuotes(t *testing.T) {
	root := t.TempDir()
	for _, body := range []string{"A=one\nA=two\n", "export A=one\n", "1KEY=value\n", "A=\"unterminated\n"} {
		writeFile(t, root, "env", body)
		if _, err := LoadEnvFile(filepath.Join(root, "env")); err == nil {
			t.Fatalf("accepted malformed env %q", body)
		}
	}
	writeFile(t, root, "env", "A=' literal ${VALUE} # value '\nB=back\\nslash\n")
	values, err := LoadEnvFile(filepath.Join(root, "env"))
	if err != nil || values["A"] != " literal ${VALUE} # value " || values["B"] != "back\\nslash" {
		t.Fatalf("literal values: %v %v", values, err)
	}
}

func TestSavedMigrationEnvPreservesLiteralQuotesAndSpaces(t *testing.T) {
	root := t.TempDir()
	values := map[string]string{"SPACES": " padded ", "QUOTES": "'literal'", "DOLLAR": "${service.web.port.80}"}
	body, err := encodeSourceEnv(values)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "migration.env", string(body))
	saved, err := LoadEnvFile(filepath.Join(root, "migration.env"))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range values {
		if saved[key] != value {
			t.Fatalf("changed saved input %s", key)
		}
	}
}
