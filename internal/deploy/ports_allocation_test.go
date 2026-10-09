package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllocatorReservesFixedAndDockerPortsAndChecksSavedOwner(t *testing.T) {
	for _, test := range []struct {
		name, ledger, listening, dockerPorts, owner string
		pending                                     []int
		want                                        string
		fail                                        bool
	}{
		{name: "pending fixed", pending: []int{18000}, want: "18001"},
		{name: "Docker DNAT", dockerPorts: "18000", want: "18001"},
		{name: "socket", listening: "18000", want: "18001"},
		{name: "reuse owned", ledger: "shop_prod:web:80\t18000\tdate\n", dockerPorts: "18000", owner: "shop_prod:web", want: "18000"},
		{name: "saved fixed conflict", ledger: "shop_prod:web:80\t18000\tdate\n", pending: []int{18000}, fail: true},
		{name: "foreign owner", ledger: "shop_prod:web:80\t18000\tdate\n", dockerPorts: "18000", owner: "other_prod:web", fail: true},
		{name: "corrupt ledger", ledger: "shop_prod:web:80\t18000\tdate\nother_prod:web:80\t18000\tdate\n", fail: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			base := filepath.Join(root, "pp")
			writeNestedFile(t, root, "pp/ports.tsv", test.ledger)
			writeExecutable(t, root, "ss", "#!/bin/sh\nif [ -n \"$PP_SOCKET\" ]; then echo \"LISTEN 0 128 127.0.0.1:$PP_SOCKET\"; fi\n")
			writeExecutable(t, root, "docker", `#!/bin/sh
case "$*" in
 'ps -q') if [ -n "$PP_DOCKER_PORT" ]; then echo container; fi;;
 'inspect '*) echo "$PP_DOCKER_PORT";;
 'ps --filter '*) echo "$PP_PORT_OWNER";;
 *) exit 1;;
esac
`)
			cmd := exec.Command("sh", "-c", autoPortScriptFor("shop_prod:web:80", test.pending, base))
			cmd.Env = append(os.Environ(), "PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "PP_SOCKET="+test.listening, "PP_DOCKER_PORT="+test.dockerPorts, "PP_PORT_OWNER="+test.owner)
			out, err := cmd.CombinedOutput()
			if (err != nil) != test.fail || (!test.fail && strings.TrimSpace(string(out)) != test.want) {
				t.Fatalf("allocation: %v %s", err, out)
			}
		})
	}
}

func TestExplicitPortRetirementPreservesOtherDeployments(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "pp")
	writeNestedFile(t, root, "pp/ports.tsv", "shop_prod:web:80\t18000\tdate\nother_prod:web:80\t18001\tdate\n")
	out, err := exec.Command("sh", "-c", releasePortsScript("shop_prod", base)).CombinedOutput()
	if err != nil {
		t.Fatalf("retirement: %v %s", err, out)
	}
	if string(mustRead(t, base+"/ports.tsv")) != "other_prod:web:80\t18001\tdate\n" {
		t.Fatal("changed another allocation")
	}
}
