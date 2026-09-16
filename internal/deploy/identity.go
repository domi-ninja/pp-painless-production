package deploy

import (
	"fmt"
	"path/filepath"
)

// DeploymentID is unambiguous because project and environment slugs cannot contain underscores.
func (p Project) DeploymentID() string {
	return p.Name + "_" + p.Environment
}

// ResourceID stays stable for adopted deployments so an upgrade does not replace volumes.
func (p Project) ResourceID() string {
	if p.resourceID != "" {
		return p.resourceID
	}
	return p.DeploymentID()
}

func (p Project) localDir(root string) string {
	return filepath.Join(root, ".deploy", p.Name, p.Environment)
}

func (p Project) remoteDir() string {
	if p.ResourceID() == p.Name {
		return ".pp/" + p.Name
	}
	return ".pp/" + p.Name + "/" + p.Environment
}

func (p Project) routePath() string {
	return "/etc/pp/proxy/routes/" + p.ResourceID() + ".caddy"
}

func (p Project) validate() error {
	if !slugPattern.MatchString(p.Name) || !slugPattern.MatchString(p.Environment) {
		return fmt.Errorf("project and environment must be lowercase DNS-safe slugs")
	}
	if p.ResourceID() != p.Name && p.ResourceID() != p.DeploymentID() {
		return fmt.Errorf("invalid resource identity %q for %s", p.ResourceID(), p.DeploymentID())
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
		if err := d.Runner.SSHScript(d.Root, host.SSH, legacyHostCheck(plan.Config.Project)); err != nil {
			return fmt.Errorf("host %s migration preflight: %w", host.ID, err)
		}
	}
	if plan.Config.Project.ResourceID() == plan.Config.Project.Name {
		for _, host := range plan.Hosts {
			if err := d.Runner.SSHScript(d.Root, host.SSH, resourceHostCheck(plan.Config.Project, true)); err != nil {
				return fmt.Errorf("host %s resource ownership: %w", host.ID, err)
			}
		}
	}
	return nil
}

func legacyHostCheck(project Project) string {
	return resourceHostCheck(project, false)
}

// A versioned host claim prevents two checkouts from adopting the same legacy
// Compose project. Claim only after every host passes the read-only preflight.
func resourceHostCheck(project Project, claim bool) string {
	return `set -eu
project=` + shellQuote(project.Name) + `
environment=` + shellQuote(project.Environment) + `
resource=` + shellQuote(project.ResourceID()) + `
claim=` + fmt.Sprint(claim) + `
owner_file=".pp/$project/resource-owner"
fail() { echo "$1" >&2; exit 45; }
[ ! -e /etc/pp/ci-runner-host ] || fail "This host is reserved for CI runners; choose an application host."
if [ "$claim" = true ]; then
  mkdir -p ".pp/$project"
  exec 8>".pp/$project/resource-owner.lock"
  flock 8
fi
owner_environment=''
if [ -f "$owner_file" ]; then
  owner=$(cat "$owner_file")
  case "$owner" in
    "1:$project:"*) owner_environment=${owner#"1:$project:"} ;;
    *) fail "Unsupported or invalid resource-owner record in $owner_file; upgrade pp or restore that record." ;;
  esac
  case "$owner_environment" in
    ''|*[!a-z0-9-]*|-*|*-) fail "Invalid environment in $owner_file; no resources changed." ;;
  esac
fi
containers=$(docker ps -aq --filter "label=com.docker.compose.project=$project")
volumes=$(docker volume ls -q --filter "label=com.docker.compose.project=$project")
if [ "$resource" = "$project" ]; then
  [ -z "$owner_environment" ] || [ "$owner_environment" = "$environment" ] || fail "Resources $project belong to $owner_environment, not $environment; no resources changed."
  scoped=$(docker ps -aq --filter "label=com.docker.compose.project=${project}_${environment}")
  [ -z "$scoped" ] || fail "Both legacy and environment-scoped deployments exist for $project/$environment; select the correct state before deploying."
  expected="$environment"
else
  if [ -z "$containers" ] && [ ! -e "/etc/pp/proxy/routes/$project.caddy" ]; then
    if [ -z "$volumes" ] || [ -f ".pp/$project/.environment-isolation-migrated" ]; then
      exit 0
    fi
  fi
  [ -n "$owner_environment" ] && [ "$owner_environment" != "$environment" ] || fail "Found unclaimed legacy resources for $project. Run pp using the original environment and its .deploy/state.json first; do not delete volumes or state."
  expected="$owner_environment"
fi
for id in $containers; do
  labels=$(docker inspect --format '{{ index .Config.Labels "pp.project" }}:{{ index .Config.Labels "pp.environment" }}' "$id")
  [ "$labels" = "$project:$expected" ] || fail "Container $id has owner $labels, expected $project:$expected; no resources changed."
done
if [ "$claim" = true ]; then
  temporary=$(mktemp ".pp/$project/.resource-owner-XXXXXX")
  trap 'rm -f "$temporary"' EXIT
  printf '1:%s:%s\n' "$project" "$environment" > "$temporary"
  mv "$temporary" "$owner_file"
fi`
}
