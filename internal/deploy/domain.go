package deploy

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sort"
	"time"
)

const domainLookupTimeout = 10 * time.Second

type hostResolver interface {
	LookupHost(context.Context, string) ([]string, error)
}

// CheckDomains prints the resolved IP addresses for every domain exposed by a deployment.
// DNS is diagnostic because a successful deployment can finish before records propagate.
func (d Deployer) CheckDomains(plan Plan) {
	resolver := d.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	for _, domain := range deploymentDomains(plan) {
		fmt.Fprintf(d.Out, "dns %s\n", domain)

		ctx, cancel := context.WithTimeout(context.Background(), domainLookupTimeout)
		addresses, err := resolver.LookupHost(ctx, domain)
		cancel()
		if err != nil {
			fmt.Fprintf(d.Out, "  lookup failed: %v\n", err)
			continue
		}

		sort.Strings(addresses)
		for _, address := range addresses {
			fmt.Fprintf(d.Out, "  %s\n", address)
		}
	}
}

func deploymentDomains(plan Plan) []string {
	domains := make([]string, 0, len(plan.Config.Routes))
	seen := make(map[string]struct{})
	add := func(domain string) {
		if domain == "" {
			return
		}
		if _, exists := seen[domain]; exists {
			return
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}

	for _, route := range plan.Config.Routes {
		add(route.Host)
	}
	if len(plan.Config.RouteFiles) == 0 {
		return domains
	}

	// Route templates do not expose their domains structurally. HTTP smoke checks
	// are the next-best source for deployments that use route_files.
	for _, check := range plan.Config.Checks["smoke"] {
		parsed, err := url.Parse(check.URL)
		if err == nil {
			add(parsed.Hostname())
		}
	}
	return domains
}
