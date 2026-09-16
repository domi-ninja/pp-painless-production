package deploy

import (
	"io"
	"testing"
)

func TestSSHTargetCannotInjectOptions(t *testing.T) {
	d := NewDeployer(t.TempDir(), io.Discard, io.Discard)
	for _, target := range []string{"-oProxyCommand=touch /tmp/marker", "-F/tmp/config", "user@host;id", "host\ncommand", "user@host:/path", "host -p 22", "$(id)"} {
		if validateSSHTarget(target) == nil {
			t.Errorf("accepted %q", target)
		}
		if d.Remote(target, "true") == nil {
			t.Fatal("Remote accepted unsafe target")
		}
		if d.Copy("unused", target, "unused") == nil {
			t.Fatal("Copy accepted unsafe target")
		}
		if d.Runner.SSHScript(d.Root, target, "true") == nil {
			t.Fatal("SSHScript accepted unsafe target")
		}
		if _, err := d.AllocateRemotePort(target, "unused"); err == nil {
			t.Fatal("allocator accepted unsafe target")
		}
	}
	for _, target := range []string{"deploy@example.com", "root@192.0.2.1", "my-ssh-alias", "user_name@host"} {
		if err := validateSSHTarget(target); err != nil {
			t.Errorf("%s: %v", target, err)
		}
	}
}
