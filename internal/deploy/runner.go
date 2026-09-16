package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Send preflight scripts on stdin so failures do not dump shell code into errors.
func (r Runner) SSHScript(dir, target, script string) error {
	if err := validateSSHTarget(target); err != nil {
		return err
	}
	cmd := exec.Command("ssh", target, "sh -s")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout, cmd.Stderr = r.Stdout, r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh %s: %w", target, err)
	}
	return nil
}

func (r Runner) Run(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func (r Runner) RunEnv(dir string, env map[string]string, name string, args ...string) error {
	return r.RunEnvContext(context.Background(), dir, env, name, args...)
}

func (r Runner) RunEnvContext(ctx context.Context, dir string, env map[string]string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = r.Stdout
	cmd.Stderr = r.Stderr
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), ctx.Err())
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func (r Runner) Output(dir string, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
