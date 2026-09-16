package deploy

import (
	"fmt"
	"strconv"
	"strings"
)

const autoPortStart = 18000
const autoPortEnd = 19999

func (d Deployer) ResolveAutoPorts(plan *Plan) error {
	for _, hostPlan := range plan.Hosts {
		for _, serviceID := range hostPlan.Services {
			service := plan.Config.Services[serviceID]
			for _, port := range service.Ports {
				if !port.Published.Auto {
					continue
				}
				key := plan.Config.Project.ResourceID() + ":" + serviceID + ":" + strconv.Itoa(port.Target)
				published, err := d.AllocateRemotePort(hostPlan.SSH, key)
				if err != nil {
					return fmt.Errorf("allocate port for %s on %s: %w", key, hostPlan.ID, err)
				}
				plan.SetAutoPort(hostPlan.ID, serviceID, port.Target, published)
			}
		}
	}
	return nil
}

func (d Deployer) AllocateRemotePort(sshTarget string, key string) (int, error) {
	if err := validateSSHTarget(sshTarget); err != nil {
		return 0, err
	}
	out, err := d.Runner.Output(d.Root, "ssh", sshTarget, autoPortScript(key))
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	portText := strings.TrimSpace(lines[len(lines)-1])
	port, err := strconv.Atoi(portText)
	if err != nil {
		return 0, fmt.Errorf("remote allocator returned %q", out)
	}
	return port, nil
}

func autoPortScript(key string) string {
	return `set -eu
mkdir -p /etc/pp
touch /etc/pp/ports.tsv
exec 9>/etc/pp/ports.lock
flock 9
key=` + shellQuote(key) + `
existing=$(awk -F '	' -v k="$key" '$1 == k { print $2; exit }' /etc/pp/ports.tsv)
if [ -n "$existing" ]; then
  echo "$existing"
  exit 0
fi
used=$( {
  awk -F '	' '{ print $2 }' /etc/pp/ports.tsv
  ss -H -ltn | awk '{ print $4 }' | sed -E 's/.*:([0-9]+)$/\1/'
} | sort -n | uniq )
for p in $(seq ` + strconv.Itoa(autoPortStart) + ` ` + strconv.Itoa(autoPortEnd) + `); do
  if printf '%s\n' "$used" | grep -qx "$p"; then
    continue
  fi
  printf '%s	%s	%s\n' "$key" "$p" "$(date -Iseconds)" >> /etc/pp/ports.tsv
  echo "$p"
  exit 0
done
echo "no free pp ports" >&2
exit 44`
}
