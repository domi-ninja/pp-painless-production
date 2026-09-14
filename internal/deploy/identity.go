package deploy

import (
	"fmt"
	"path/filepath"
)

// DeploymentID is unambiguous because project and environment slugs cannot contain underscores.
func (p Project) DeploymentID() string {
	return p.Name + "_" + p.Environment
}

func (p Project) localDir(root string) string {
	return filepath.Join(root, ".deploy", p.Name, p.Environment)
}

func (p Project) remoteDir() string {
	return ".pp/" + p.Name + "/" + p.Environment
}

func (p Project) routePath() string {
	return "/etc/pp/proxy/routes/" + p.DeploymentID() + ".caddy"
}

func (p Project) validate() error {
	if !slugPattern.MatchString(p.Name) || !slugPattern.MatchString(p.Environment) {
		return fmt.Errorf("project and environment must be lowercase DNS-safe slugs")
	}
	return nil
}

func (p Project) checkOwner(name, environment string) error {
	if p.Name != name || p.Environment != environment {
		return fmt.Errorf("deployment ownership mismatch: selected %s/%s, record belongs to %s/%s", p.Name, p.Environment, name, environment)
	}
	return nil
}

// Preflight all hosts before builds, migrations or port allocation can change anything.
func (d Deployer) checkLegacyHosts(plan Plan) error {
	for _, host := range plan.Hosts {
		if err := d.Remote(host.SSH, legacyHostCheck(plan.Config.Project)); err != nil {
			return fmt.Errorf("host %s migration preflight: %w", host.ID, err)
		}
	}
	return nil
}

func legacyHostCheck(project Project) string {
	filter := shellQuote("label=com.docker.compose.project=" + project.Name)
	return "set -eu\n" +
		"containers=$(docker ps -aq --filter " + filter + ")\n" +
		"volumes=$(docker volume ls -q --filter " + filter + ")\n" +
		"if [ -n \"$containers\" ] || [ -e " + shellQuote("/etc/pp/proxy/routes/"+project.Name+".caddy") +
		" ] || { [ -n \"$volumes\" ] && [ ! -f " + shellQuote(".pp/"+project.Name+"/.environment-isolation-migrated") + " ]; }; then\n" +
		"  echo " + shellQuote("legacy deployment for "+project.Name+" requires explicit migration; see deploy-cli.md") + " >&2\n" +
		"  exit 45\nfi"
}
