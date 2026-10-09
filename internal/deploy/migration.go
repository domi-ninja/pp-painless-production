package deploy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
)

// Create before attaching so cancellation can remove the known owned container.
func (d Deployer) runMigration(migration *Migration, image string, rollback bool) (result error) {
	command := migration.Command
	if rollback {
		command = migration.RollbackCommand
	}
	if len(command) == 0 {
		if rollback && migration.RestorePlan != "" {
			return fmt.Errorf("restore plan requires manual action: %s", migration.RestorePlan)
		}
		return fmt.Errorf("migration command is empty")
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	name := "pp-migration-" + hex.EncodeToString(token[:])
	args := []string{"create", "--name", name, "--label", "pp.migration=" + name, "--pull", "never"}
	if migration.Env.Source != "" {
		values, err := loadSelectedEnv(d.Root, migration.Env)
		if err != nil {
			return err
		}
		body, err := encodeEnv(values)
		if err != nil {
			return err
		}
		file, err := os.CreateTemp("", "pp-migration-env-")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		if _, err := file.Write(body); err != nil {
			file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		args = append(args, "--env-file", file.Name())
	}
	args = append(args, image)
	args = append(args, command...)
	ctx, cancel := optionalTimeoutContext(d.Runner.context(), migration.TimeoutSeconds)
	defer cancel()
	defer func() { result = errors.Join(result, d.removeMigrationContainer(name)) }()
	if err := d.Runner.RunEnvContext(ctx, d.Root, nil, "docker", args...); err != nil {
		return err
	}
	return d.Runner.RunEnvContext(ctx, d.Root, nil, "docker", "start", "--attach", name)
}

func (d Deployer) removeMigrationContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(d.Runner.context()), 15*time.Second)
	defer cancel()
	runner := d.Runner
	runner.Context = ctx
	script := `set -eu
name="$1"
present="$(docker ps -a --filter name="^/$name$" --format '{{.Names}}')"
[ -n "$present" ] || exit 0
[ "$present" = "$name" ] || { echo 'Ambiguous migration container' >&2; exit 1; }
owner="$(docker inspect --format '{{index .Config.Labels "pp.migration"}}' "$name")"
[ "$owner" = "$name" ] || { echo 'Migration container ownership changed' >&2; exit 1; }
docker rm -f "$name" >/dev/null`
	if err := runner.RunEnvContext(ctx, d.Root, nil, "sh", "-c", script, "pp-cleanup", name); err != nil {
		return fmt.Errorf("remove migration container %s: %w", name, err)
	}
	return nil
}
