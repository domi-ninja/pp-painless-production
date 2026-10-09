package deploy

import (
	"fmt"
	"strconv"
	"strings"
)

const autoPortStart = 18000
const autoPortEnd = 19999

func (d Deployer) ResolveAutoPorts(plan *Plan) error {
	for _, host := range plan.Hosts {
		var pending []int
		for _, id := range host.Services {
			for _, port := range plan.Config.Services[id].Ports {
				if !port.Published.Auto {
					pending = append(pending, port.Published.Value)
				}
			}
		}
		resolved := map[int]string{}
		for _, id := range host.Services {
			for _, port := range plan.Config.Services[id].Ports {
				key := plan.Config.Project.ResourceID() + ":" + id + ":" + strconv.Itoa(port.Target)
				published := port.Published.Value
				if port.Published.Auto {
					var err error
					published, err = d.allocateRemotePort(host.SSH, key, pending)
					if err != nil {
						return fmt.Errorf("allocate port for %s on %s: %w", key, host.ID, err)
					}
					plan.SetAutoPort(host.ID, id, port.Target, published)
				} else if err := d.Runner.SSHScript(d.Root, host.SSH, portOwnershipScript(key, published, "/etc/pp")); err != nil {
					return fmt.Errorf("fixed port for %s on %s: %w", id, host.ID, err)
				}
				if owner, ok := resolved[published]; ok {
					return fmt.Errorf("resolved port %d conflicts between %s and %s on %s", published, owner, id, host.ID)
				}
				resolved[published] = id
			}
		}
	}
	return nil
}

func (d Deployer) AllocateRemotePort(target, key string) (int, error) {
	return d.allocateRemotePort(target, key, nil)
}
func (d Deployer) allocateRemotePort(target, key string, pending []int) (int, error) {
	if err := validateSSHTarget(target); err != nil {
		return 0, err
	}
	out, err := d.Runner.Output(d.Root, "ssh", target, autoPortScriptFor(key, pending, "/etc/pp"))
	if err != nil {
		return 0, err
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	port, err := strconv.Atoi(strings.TrimSpace(lines[len(lines)-1]))
	if err != nil || port < autoPortStart || port > autoPortEnd {
		return 0, fmt.Errorf("remote allocator returned an invalid port")
	}
	return port, nil
}
func autoPortScript(key string) string { return autoPortScriptFor(key, nil, "/etc/pp") }

func portInputs(key, base string) string {
	return `set -eu
base=` + shellQuote(base) + `
key=` + shellQuote(key) + `
resource=${key%%:*}
rest=${key#*:}
service=${rest%%:*}
[ ! -L "$base" ] && [ ! -L "$base/ports.tsv" ] && [ ! -L "$base/ports.lock" ]
mkdir -p "$base"
touch "$base/ports.tsv"
exec 9>"$base/ports.lock"
flock 9
awk -F '\t' 'NF != 3 || $1 !~ /^[a-z0-9][a-z0-9_-]*:[a-z0-9][a-z0-9-]*:[0-9]+$/ || $2 !~ /^[0-9]+$/ || $2 < 1 || $2 > 65535 || keys[$1]++ || ports[$2]++ {exit 1}' "$base/ports.tsv" || { echo 'Invalid or conflicting saved port map; inspect it before retrying' >&2; exit 1; }
sockets=$(ss -H -ltn)
listening=$(printf '%s\n' "$sockets" | awk '{print $4}' | sed -E 's/.*:([0-9]+)$/\1/')
ids=$(docker ps -q)
docker_ports=''
if [ -n "$ids" ]; then docker_ports=$(docker inspect --format '{{range .NetworkSettings.Ports}}{{range .}}{{println .HostPort}}{{end}}{{end}}' $ids); fi
check_owner() {
 port=$1
 reserved=$(awk -F '\t' -v p="$port" '$2 == p {print $1}' "$base/ports.tsv")
 [ -z "$reserved" ] || [ "$reserved" = "$key" ] || { echo 'Port is reserved for another deployment or service' >&2; exit 1; }
 if printf '%s\n%s\n' "$listening" "$docker_ports" | grep -qx "$port"; then
  owners=$(docker ps --filter "publish=$port" --format '{{.Label "com.docker.compose.project"}}:{{.Label "pp.service"}}' | sort -u)
  [ "$owners" = "$resource:$service" ] || { echo 'Port is used by another deployment or process; no allocation changed' >&2; exit 1; }
 fi
}
`
}

func autoPortScriptFor(key string, pending []int, base string) string {
	var ports []string
	for _, port := range pending {
		ports = append(ports, strconv.Itoa(port))
	}
	return portInputs(key, base) + `
pending=` + shellQuote(strings.Join(ports, "\n")) + `
existing=$(awk -F '\t' -v k="$key" '$1 == k {print $2}' "$base/ports.tsv")
if [ -n "$existing" ]; then
 [ "$existing" -ge ` + strconv.Itoa(autoPortStart) + ` ] && [ "$existing" -le ` + strconv.Itoa(autoPortEnd) + ` ] || { echo 'Saved automatic port is outside the supported range' >&2; exit 1; }
 if printf '%s\n' "$pending" | grep -qx "$existing"; then echo 'Saved automatic port conflicts with a fixed port in this plan' >&2; exit 1; fi
 check_owner "$existing"
 echo "$existing"
 exit 0
fi
used=$( { awk -F '\t' '{print $2}' "$base/ports.tsv"; printf '%s\n%s\n%s\n' "$listening" "$docker_ports" "$pending"; } | sort -nu)
for p in $(seq ` + strconv.Itoa(autoPortStart) + ` ` + strconv.Itoa(autoPortEnd) + `); do
 if printf '%s\n' "$used" | grep -qx "$p"; then continue; fi
 printf '%s\t%s\t%s\n' "$key" "$p" "$(date -Iseconds)" >> "$base/ports.tsv"
 echo "$p"
 exit 0
done
echo 'No free pp ports' >&2
exit 44
`
}
func portOwnershipScript(key string, port int, base string) string {
	return portInputs(key, base) + "\ncheck_owner " + strconv.Itoa(port) + "\n"
}
func releasePortsScript(resource, base string) string {
	return `set -eu
base=` + shellQuote(base) + `
prefix=` + shellQuote(resource+":") + `
[ ! -e "$base/ports.tsv" ] && exit 0
[ ! -L "$base/ports.tsv" ] && [ ! -L "$base/ports.lock" ]
exec 9>"$base/ports.lock"
flock 9
file=$(mktemp "$base/.ports.XXXXXX")
trap 'rm -f "$file"' EXIT HUP INT TERM
awk -F '\t' -v prefix="$prefix" 'substr($1,1,length(prefix)) != prefix' "$base/ports.tsv" > "$file"
chmod 0600 "$file"
mv "$file" "$base/ports.tsv"
`
}
