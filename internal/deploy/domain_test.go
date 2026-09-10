package deploy

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type stubHostResolver struct {
	addresses map[string][]string
	errors    map[string]error
}

func (r stubHostResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	return r.addresses[host], r.errors[host]
}

func TestDeploymentDomainsUsesConfiguredRoutes(t *testing.T) {
	plan := Plan{Config: Config{
		Routes: []Route{
			{Host: "app.example.com"},
			{Host: "api.example.com"},
			{Host: "app.example.com"},
		},
		Checks: map[string][]Check{
			"smoke": {{URL: "https://ignored.example.com/health"}},
		},
	}}

	got := deploymentDomains(plan)
	want := []string{"app.example.com", "api.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deploymentDomains() = %#v, want %#v", got, want)
	}
}

func TestDeploymentDomainsIncludesHTTPSmokeChecksForRouteFiles(t *testing.T) {
	plan := Plan{Config: Config{
		Routes:     []Route{{Host: "app.example.com"}},
		RouteFiles: []RouteFile{{Source: "deploy/app.caddy", Host: "server"}},
		Checks: map[string][]Check{
			"smoke": {
				{URL: "https://app.example.com/"},
				{URL: "https://api.example.com/version"},
				{URL: "https://app.example.com/health"},
				{Command: []string{"true"}},
			},
		},
	}}

	got := deploymentDomains(plan)
	want := []string{"app.example.com", "api.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("deploymentDomains() = %#v, want %#v", got, want)
	}
}

func TestCheckDomainsPrintsSortedAnswersAndLookupFailures(t *testing.T) {
	plan := Plan{Config: Config{Routes: []Route{
		{Host: "app.example.com"},
		{Host: "missing.example.com"},
	}}}
	var out bytes.Buffer
	deployer := Deployer{
		Out: &out,
		resolver: stubHostResolver{
			addresses: map[string][]string{
				"app.example.com": {"2001:db8::1", "192.0.2.1"},
			},
			errors: map[string]error{
				"missing.example.com": errors.New("no such host"),
			},
		},
	}

	deployer.CheckDomains(plan)

	want := strings.Join([]string{
		"dns app.example.com",
		"  192.0.2.1",
		"  2001:db8::1",
		"dns missing.example.com",
		"  lookup failed: no such host",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("unexpected DNS output:\n%s\nwant:\n%s", out.String(), want)
	}
}
