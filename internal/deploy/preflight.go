package deploy

import (
	"fmt"
	"os/exec"
	"strings"
)

func (d Deployer) preflight(plan Plan) error {
	for _, name := range []string{"ssh", "scp"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("required tool %s: %w", name, err)
		}
	}
	localPlatform := ""
	if plan.Config.Build.Context != "" || len(plan.Config.Builds) > 0 || plan.Config.Migrations != nil {
		info, err := d.Runner.Output(d.Root, "docker", "info", "--format", "{{.OSType}}/{{.Architecture}}")
		if err != nil {
			return err
		}
		parts := strings.Split(info, "/")
		if len(parts) != 2 || parts[0] != "linux" {
			return fmt.Errorf("local Docker must use Linux containers")
		}
		localPlatform = "linux/" + normalizeArchitecture(parts[1])
	}
	if plan.Config.Build.Context != "" || len(plan.Config.Builds) > 0 {
		help, err := d.Runner.Output(d.Root, "docker", "buildx", "prune", "--help")
		if err != nil {
			return err
		}
		for _, flag := range []string{"--max-used-space", "--reserved-space", "--min-free-space"} {
			if !strings.Contains(help, flag) {
				return fmt.Errorf("Docker Buildx needs %s support; update Buildx", flag)
			}
		}
	}
	for _, host := range plan.Hosts {
		needsRoutes := false
		for _, route := range plan.Config.Routes {
			if routeBelongsToHost(plan.Config, route, host.ID) {
				needsRoutes = true
			}
		}
		for _, route := range plan.Config.RouteFiles {
			if route.Host == host.ID {
				needsRoutes = true
			}
		}
		platforms := map[string]bool{}
		for _, id := range host.Services {
			service := plan.Config.Services[id]
			build := plan.Config.Build
			if len(plan.Config.Builds) > 0 {
				if service.Build == "" {
					continue
				}
				build = plan.Config.Builds[service.Build]
			}
			if build.Context == "" {
				continue
			}
			platform := localPlatform
			if len(build.Platforms) > 0 {
				platform = build.Platforms[0]
			}
			platforms[platform] = true
		}
		var expected []string
		for _, item := range sortedMap(platforms) {
			expected = append(expected, item.Key)
		}
		if err := d.Runner.SSHScript(d.Root, host.SSH, hostPreflightScript("/etc/pp", needsRoutes, expected)); err != nil {
			return fmt.Errorf("host %s preflight: %w", host.ID, err)
		}
	}
	return nil
}

func normalizeArchitecture(arch string) string {
	switch arch {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	}
	return arch
}

func hostPreflightScript(base string, routes bool, platforms []string) string {
	script := `set -eu
base=` + shellQuote(base) + `
fail() { echo "$1" >&2; exit 46; }
for tool in docker flock ss df; do command -v "$tool" >/dev/null || fail "Missing host tool: $tool"; done
docker info >/dev/null || fail 'Deployment account cannot access Docker'
version=$(docker compose version --short | sed 's/^v//')
printf '%s\n' "$version" | awk -F. '{exit !($1 > 2 || ($1 == 2 && $2 >= 30))}' || fail 'Host needs Compose 2.30 or newer'
[ -w . ] || fail 'Deployment account needs a writable home directory'
[ -d "$base" ] && [ -w "$base" ] && [ ! -L "$base" ] || fail 'Deployment account needs an owned /etc/pp directory'
[ ! -L "$base/ports.tsv" ] && [ ! -L "$base/ports.lock" ] || fail 'Port files must not be symlinks'
if [ -e "$base/ports.tsv" ]; then [ -w "$base/ports.tsv" ] || fail 'Port map is not writable'; fi
`
	if routes {
		script += `command -v caddy >/dev/null || fail 'Public routes need Caddy'
[ -d "$base/proxy/routes" ] && [ -w "$base/proxy/routes" ] || fail 'Deployment account needs writable pp proxy routes'
[ ! -L "$base/proxy" ] && [ ! -L "$base/proxy/routes" ] || fail 'Proxy directories must not be symlinks'
caddy adapt --adapter caddyfile --config /etc/caddy/Caddyfile >/dev/null || fail 'Cannot read or adapt the host Caddyfile'
curl --silent --fail --max-time 10 http://localhost:2019/config/ >/dev/null || fail 'Caddy local admin endpoint is unavailable'
`
	}
	if len(platforms) > 0 {
		script += `arch=$(uname -m)
case "$arch" in x86_64) arch=amd64;; aarch64) arch=arm64;; esac
` + "for platform in " + shellJoin(platforms) + `; do
 [ "$platform" = "linux/$arch" ] || fail "Built platform $platform does not match host linux/$arch"
done
`
	}
	return script
}
