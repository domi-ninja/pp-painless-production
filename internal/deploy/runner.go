package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Runner struct {
	Context context.Context
	Stdout  io.Writer
	Stderr  io.Writer
}

func (r Runner) context() context.Context {
	if r.Context != nil {
		return r.Context
	}
	return context.Background()
}

// Send preflight scripts on stdin so failures do not dump shell code into errors.
func (r Runner) SSHScript(dir, target, script string) error {
	if err := validateSSHTarget(target); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(r.context(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", transportArgs("ssh", []string{target, "sh -s"})...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout, cmd.Stderr = r.Stdout, r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh %s: %w", target, err)
	}
	return nil
}

func (r Runner) Run(dir string, name string, args ...string) error {
	ctx, cancel := r.transportContext(name)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, transportArgs(name, args)...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func (r Runner) RunEnv(dir string, env map[string]string, name string, args ...string) error {
	return r.RunEnvContext(context.Background(), dir, env, name, args...)
}

func (r Runner) RunEnvContext(ctx context.Context, dir string, env map[string]string, name string, args ...string) error {
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.context(), cancel)
	defer stop()
	defer cancel()
	cmd := exec.CommandContext(ctx, name, transportArgs(name, args)...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s failed: %w", name, ctx.Err())
		}
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}

func (r Runner) Output(dir string, name string, args ...string) (string, error) {
	ctx, cancel := r.transportContext(name)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, transportArgs(name, args)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s failed: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func transportArgs(name string, args []string) []string {
	if name != "ssh" && name != "scp" {
		return args
	}
	return append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=2"}, args...)
}
func (r Runner) transportContext(name string) (context.Context, context.CancelFunc) {
	if name == "ssh" || name == "scp" {
		return context.WithTimeout(r.context(), 10*time.Minute)
	}
	return context.WithCancel(r.context())
}
