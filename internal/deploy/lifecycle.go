package deploy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func (d Deployer) operationLock() (func(), error) {
	if err := noSymlinkPath(d.Root, ".deploy"); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(d.Root, ".deploy"), 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(d.Root, ".deploy", ".operation.lock")
	if err := noSymlinkPath(d.Root, ".deploy/.operation.lock"); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another pp operation is running in this checkout")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

// Keep SSH stdin open for the entire operation. The server releases flock if
// the process or connection dies. A competing checkout fails rather than races.
func (d *Deployer) lockHosts(plan Plan, rollback bool) (func(), error) {
	ctx, cancel := context.WithCancel(d.Runner.context())
	d.Runner.Context = ctx
	targets := map[string]bool{}
	for _, host := range plan.Hosts {
		targets[host.SSH] = true
	}
	records, err := d.releaseRecords(plan.Config.Project)
	if err != nil {
		cancel()
		return nil, err
	}
	state, err := LoadState(d.Root, plan.Config.Project)
	if err != nil {
		cancel()
		return nil, err
	}
	for _, record := range records {
		if record.ReleaseID == state.CurrentReleaseID || (rollback && record.ReleaseID == state.PreviousReleaseID) {
			for _, host := range record.Hosts {
				targets[host.SSH] = true
			}
		}
	}
	var names []string
	for target := range targets {
		names = append(names, target)
	}
	sort.Strings(names)
	var closes []func()
	unlock := func() {
		for i := len(closes) - 1; i >= 0; i-- {
			closes[i]()
		}
		cancel()
	}
	d.lockedHosts = map[string]bool{}
	for _, target := range names {
		if err := validateSSHTarget(target); err != nil {
			unlock()
			return nil, err
		}
		script := remoteLockScript() + "printf 'pp-lock-ready\\n'; cat >/dev/null"
		cmd := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2", target, script)
		var lockErrors bytes.Buffer
		cmd.Dir, cmd.Stderr = d.Root, &lockErrors
		stdin, err := cmd.StdinPipe()
		if err != nil {
			unlock()
			return nil, err
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			stdin.Close()
			unlock()
			return nil, err
		}
		if err := cmd.Start(); err != nil {
			stdin.Close()
			unlock()
			return nil, err
		}
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil || strings.TrimSpace(line) != "pp-lock-ready" {
			stdin.Close()
			_ = cmd.Wait()
			unlock()
			return nil, fmt.Errorf("cannot lock deployment host %s; another pp operation may be running: %s", target, strings.TrimSpace(lockErrors.String()))
		}
		d.lockedHosts[target] = true
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); cancel(); close(done) }()
		closes = append(closes, func() {
			_ = stdin.Close()
			<-done
			if lockErrors.Len() > 0 {
				fmt.Fprintf(d.Err, "host lock %s: %s\n", target, strings.TrimSpace(lockErrors.String()))
			}
			delete(d.lockedHosts, target)
		})
	}
	return unlock, nil
}

func remoteLockScript() string {
	return "set -eu; umask 077; [ ! -L .pp ] && mkdir -p .pp; [ ! -L .pp/operation.lock ]; exec 9>.pp/operation.lock; flock -n 9 || { echo 'another pp operation is running on this host' >&2; exit 1; }; "
}

func (d Deployer) checkSpace(plan Plan) error {
	minimum := plan.Config.Retention.defaults().MinFreeMB * 1024 * 1024
	var stat syscall.Statfs_t
	if err := syscall.Statfs(d.Root, &stat); err != nil {
		return err
	}
	if uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(minimum) {
		return fmt.Errorf("less than %d MiB free locally; run pp cleanup or free disk space", minimum/(1024*1024))
	}
	// Docker Desktop's data root is inside its VM, not on the client filesystem.
	return d.Runner.Run(d.Root, "sh", "-c", "set -eu; docker_root=$(docker info --format '{{.DockerRootDir}}'); if [ -d \"$docker_root\" ]; then\n"+spaceScript("$docker_root", minimum)+"\nfi")
}

func (d Deployer) checkExportSpace(plan Plan, image ImageBundle) error {
	out, err := d.Runner.Output(d.Root, "docker", "image", "inspect", "--format", "{{.Size}}", image.Tags[0])
	if err != nil {
		return err
	}
	size, err := strconv.ParseInt(out, 10, 64)
	if err != nil || size < 0 || size > 1<<60 {
		return fmt.Errorf("invalid Docker image size")
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(d.Root, &stat); err != nil {
		return err
	}
	needed := 2*size + 16*1024*1024 + plan.Config.Retention.defaults().MinFreeMB*1024*1024
	if uint64(stat.Bavail)*uint64(stat.Bsize) < uint64(needed) {
		return fmt.Errorf("insufficient local space to export image; need %d MiB including reserve", needed/(1024*1024))
	}
	return nil
}

// df -Pk reports portable 1 KiB blocks. Arguments here are generated, not raw
// configuration. Check the actual Docker filesystem as well as bundle storage.
func spaceScript(path string, bytes int64) string {
	return fmt.Sprintf("set -eu; available=$(df -Pk \"%s\" | awk 'END {print $4}'); [ \"$available\" -ge %d ] || { echo 'insufficient free space for pp deployment' >&2; exit 1; }", path, (bytes+1023)/1024)
}
