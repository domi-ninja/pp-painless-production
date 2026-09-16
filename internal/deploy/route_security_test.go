package deploy

import "testing"

func TestCaddyRouteRejectsDirectiveInjection(t *testing.T) {
	for _, host := range []string{"site.test\n{\nrespond hacked\n}", "site.test other.test", "site.test{", "site.test#", "site.test:443", "*.*.test"} {
		if validateCaddyHost(host) == nil {
			t.Errorf("accepted host %q", host)
		}
	}
	for _, host := range []string{"site.test", "*.site.test", "localhost", "127.0.0.1", "EXAMPLE.COM."} {
		if err := validateCaddyHost(host); err != nil {
			t.Errorf("host %q: %v", host, err)
		}
	}
	for _, target := range []string{"http://localhost:3000/\n}\nrespond hacked", "http://localhost:3000/{env.SECRET}", "http://user:pass@localhost:3000", "http://localhost:3000/#comment", "http://localhost:3000?x=1"} {
		if validateCaddyTarget(target) == nil {
			t.Errorf("accepted target %q", target)
		}
	}
	for _, target := range []string{"http://127.0.0.1:3000", "https://backend.test", "http://[::1]:3000"} {
		if err := validateCaddyTarget(target); err != nil {
			t.Errorf("target %q: %v", target, err)
		}
	}
}
