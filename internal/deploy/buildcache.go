package deploy

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// One dedicated builder per local user. Never prune the user's default builder.
// The global lock also prevents one checkout pruning another checkout's build.
func (d Deployer) prepareBuilder(create bool) (string, func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	if err := noSymlinkPath(home, ".pp/buildkit.lock"); err != nil {
		return "", nil, err
	}
	dir := filepath.Join(home, ".pp")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "buildkit.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return "", nil, fmt.Errorf("another pp build is running for this user")
	}
	unlock := func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }
	name := fmt.Sprintf("pp-managed-%x", sha256.Sum256([]byte(home)))[:23]
	marker := filepath.Join(dir, name+".owner")
	if err := noSymlinkPath(home, ".pp/"+name+".owner"); err != nil {
		unlock()
		return "", nil, err
	}
	owned, err := os.ReadFile(marker)
	if err != nil && !os.IsNotExist(err) {
		unlock()
		return "", nil, err
	}
	driver, inspectErr := d.Runner.Output(d.Root, "docker", "buildx", "inspect", name)
	if inspectErr == nil {
		managed := false
		for _, line := range strings.Split(driver, "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && fields[0] == "Driver:" && fields[1] == "docker-container" {
				managed = true
			}
		}
		if string(owned) != "pp build cache v1\n" || !managed {
			unlock()
			return "", nil, fmt.Errorf("builder %s exists but is not pp-owned", name)
		}
	} else {
		if !create {
			return "", unlock, nil
		}
		if err := d.Runner.Run(d.Root, "docker", "buildx", "create", "--name", name, "--driver", "docker-container"); err != nil {
			unlock()
			return "", nil, err
		}
		if err := os.WriteFile(marker, []byte("pp build cache v1\n"), 0600); err != nil {
			unlock()
			return "", nil, err
		}
	}
	if err := d.Runner.Run(d.Root, "docker", "buildx", "inspect", name, "--bootstrap"); err != nil {
		unlock()
		return "", nil, err
	}
	return name, unlock, nil
}

func (d Deployer) cleanupBuildCache(retention Retention, dryRun bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	name := fmt.Sprintf("pp-managed-%x", sha256.Sum256([]byte(home)))[:23]
	marker := filepath.Join(home, ".pp", name+".owner")
	if _, err := os.Stat(marker); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "cleanup: pp build cache target %d MiB\n", retention.defaults().BuildCacheMB)
	if dryRun {
		return nil
	}
	builder, unlock, err := d.prepareBuilder(false)
	if err != nil {
		return err
	}
	defer unlock()
	if builder == "" {
		return nil
	}
	return d.trimBuildCache(builder, retention)
}

func (d Deployer) trimBuildCache(builder string, retention Retention) error {
	r := retention.defaults()
	return d.Runner.Run(d.Root, "docker", "buildx", "prune", "--builder", builder, "--all", "--force", "--max-used-space", fmt.Sprint(r.BuildCacheMB*1024*1024), "--reserved-space", "0", "--min-free-space", fmt.Sprint(r.MinFreeMB*1024*1024))
}
