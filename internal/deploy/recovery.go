package deploy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// The contract describes recovery, not a second copy of the current app config.
// Scripts in the checkout are never assumed to belong to an older release.
type recoveryContract struct {
	RouteHosts       []string           `json:"route_hosts,omitempty"`
	Version          int                `json:"version"`
	Services         map[string]Service `json:"services"`
	Checks           map[string][]Check `json:"checks,omitempty"`
	Migration        *Migration         `json:"migration,omitempty"`
	HasHooks         bool               `json:"has_hooks,omitempty"`
	HasCommandChecks bool               `json:"has_command_checks,omitempty"`
}

func saveRecoveryContract(root string, plan Plan, bundle Bundle) error {
	publicValues, err := loadConfigEnv(root, plan.Config.Env)
	if err != nil {
		return err
	}
	contract := recoveryContract{Version: 1, Services: map[string]Service{}, Checks: map[string][]Check{}}
	for _, host := range bundle.Hosts {
		if host.Routes != "" {
			contract.RouteHosts = append(contract.RouteHosts, host.ID)
		}
	}
	for id, svc := range plan.Config.Services {
		contract.Services[id] = Service{Phase: svc.Phase, Hosts: svc.Hosts}
	}
	for _, hooks := range plan.Config.Hooks {
		if len(hooks) > 0 {
			contract.HasHooks = true
		}
	}
	for group, checks := range plan.Config.Checks {
		for _, check := range checks {
			if len(check.Command) > 0 {
				contract.HasCommandChecks = true
				continue
			}
			check.URL = RenderValue(check.URL, plan, "", publicValues)
			contract.Checks[group] = append(contract.Checks[group], check)
		}
	}
	if plan.Config.Migrations != nil {
		migration := *plan.Config.Migrations
		migration.Image = RenderValue(migration.Image, plan, "", publicValues)
		values, err := loadSelectedEnv(root, migration.Env)
		if err != nil {
			return err
		}
		migration.Command = renderArgs(migration.Command, plan, "", values)
		migration.RollbackCommand = renderArgs(migration.RollbackCommand, plan, "", values)
		body, err := encodeSourceEnv(values)
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(bundle.Root, "migration.env"), body); err != nil {
			return err
		}
		// Resolved relative to the release directory when loaded, including moved checkouts.
		migration.Env = EnvSpec{Source: "migration.env", IncludeAll: true}
		contract.Migration = &migration
	}
	return writeJSON(filepath.Join(bundle.Root, "recovery.json"), contract)
}

func recoveryPlan(root string, record ReleaseRecord) (Plan, error) {
	project := Project{Name: record.Project, Environment: record.Environment, resourceID: record.ResourceID}
	plan := Plan{Config: Config{Project: project, Services: map[string]Service{}, Hosts: map[string]Host{}}, ReleaseID: record.ReleaseID, Git: record.Git}
	contract, err := readRecoveryContract(root, record.BundlePath)
	if err == nil {
		if contract.HasHooks || contract.HasCommandChecks {
			return Plan{}, fmt.Errorf("release %s uses application hooks or command checks; recover those application inputs explicitly before restoring containers", record.ReleaseID)
		}
		plan.Config.Services, plan.Config.Checks = contract.Services, contract.Checks
		plan.Config.Migrations, err = resolveSavedMigration(root, record.BundlePath, contract.Migration)
		if err != nil {
			return Plan{}, err
		}
		for _, hostID := range contract.RouteHosts {
			if !slugPattern.MatchString(hostID) {
				return Plan{}, fmt.Errorf("invalid saved route host")
			}
			path := filepath.Join(record.BundlePath, "hosts", hostID, "routes.caddy")
			if err := validateRecoveryArtifact(root, record.BundlePath, path); err != nil {
				return Plan{}, err
			}
			if _, err := os.ReadFile(path); err != nil {
				return Plan{}, fmt.Errorf("required recovery route file: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return Plan{}, err
	} else if record.Migration.Status != "" && record.Migration.Status != "skipped" {
		return Plan{}, fmt.Errorf("release %s has no saved migration contract; database recovery needs manual action", record.ReleaseID)
	}
	for _, host := range record.Hosts {
		if !slugPattern.MatchString(host.ID) {
			return Plan{}, fmt.Errorf("invalid saved host ID")
		}
		if err := validateSSHTarget(host.SSH); err != nil {
			return Plan{}, err
		}
		plan.Config.Hosts[host.ID] = Host{SSH: host.SSH}
		plan.Hosts = append(plan.Hosts, HostPlan{ID: host.ID, SSH: host.SSH, Services: host.Services})
		path := filepath.Join(record.BundlePath, "hosts", host.ID, "compose.yml")
		if err := validateRecoveryArtifact(root, record.BundlePath, path); err != nil {
			return Plan{}, err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return Plan{}, fmt.Errorf("required recovery Compose file: %w", err)
		}
		var compose ComposeFile
		if err := yaml.Unmarshal(body, &compose); err != nil {
			return Plan{}, err
		}
		manifestPath := filepath.Join(filepath.Dir(path), "images.json")
		if err := validateRecoveryArtifact(root, record.BundlePath, manifestPath); err != nil {
			return Plan{}, err
		}
		manifest, err := readImageManifest(manifestPath)
		if err != nil {
			return Plan{}, err
		}
		for id, service := range compose.Services {
			if !slugPattern.MatchString(id) {
				return Plan{}, fmt.Errorf("invalid saved service ID")
			}
			if strings.HasPrefix(service.Image, managedImagePrefix(project)) {
				locked := false
				for _, image := range manifest.Images {
					if image.Reference == service.Image {
						locked = true
					}
				}
				if !locked {
					return Plan{}, fmt.Errorf("required recovery image lock is missing for %s", id)
				}
			}
			for _, env := range service.EnvFile {
				path := filepath.Join(filepath.Dir(path), env.Path)
				if err := validateRecoveryArtifact(root, record.BundlePath, path); err != nil {
					return Plan{}, err
				}
				if _, err := os.ReadFile(path); err != nil {
					return Plan{}, fmt.Errorf("required recovery env file: %w", err)
				}
			}
		}
	}
	for _, image := range bundleFromRecord(record).Images {
		if err := validateRecoveryArtifact(root, record.BundlePath, image.Tar); err != nil {
			return Plan{}, err
		}
		if info, err := os.Stat(image.Tar); err != nil || !info.Mode().IsRegular() {
			return Plan{}, fmt.Errorf("required recovery image archive is missing: %s", image.ID)
		}
	}
	return plan, nil
}

func validateRecoveryArtifact(root, bundle, path string) error {
	relative, err := filepath.Rel(bundle, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("recovery artifact is outside its release")
	}
	relative, err = filepath.Rel(root, path)
	if err != nil {
		return err
	}
	return noSymlinkPath(root, relative)
}

// Validate the target first. Revert a completed database migration before older
// code starts; a restore plan or failed revert prevents container recovery.
func (d Deployer) recoverRelease(current ReleaseRecord, target *ReleaseRecord, revertDatabase bool) (StepRecord, []CheckRecord, error) {
	if target == nil && !revertDatabase {
		return StepRecord{Status: "skipped", At: time.Now().UTC()}, nil, nil
	}
	step := StepRecord{Status: "running", StartedAt: time.Now().UTC()}
	var checks []CheckRecord
	d.onHostStep = nil
	if target != nil {
		d.onHostStep = d.recordHostProgress(target)
	}
	run := func() error {
		if target != nil && target.CreatedAt.After(current.CreatedAt) && (current.Migration.Status == "ok" || target.Migration.Status == "ok") {
			return fmt.Errorf("target release is newer and includes database migrations; redeploy the intended version with its forward migration policy")
		}
		var plan Plan
		if target != nil {
			var err error
			plan, err = recoveryPlan(d.Root, *target)
			if err != nil {
				return err
			}
		}
		if revertDatabase {
			currentPlan, err := recoveryPlan(d.Root, current)
			if err != nil {
				return err
			}
			migration := currentPlan.Config.Migrations
			if migration == nil || !imageIDPattern.MatchString(current.Migration.Image) {
				return fmt.Errorf("no saved database recovery command or image; manual recovery required")
			}
			if _, err := d.Runner.Output(d.Root, "docker", "image", "inspect", current.Migration.Image); err != nil {
				return fmt.Errorf("saved migration image is unavailable: %w", err)
			}
			fmt.Fprintln(d.Out, "running saved database rollback before container recovery")
			if err := d.runMigration(migration, current.Migration.Image, true); err != nil {
				return err
			}
		}
		if target != nil {
			var err error
			checks, err = d.ApplyRelease(plan, bundleFromRecord(*target))
			return err
		}
		return nil
	}
	err := run()
	step.FinishedAt = time.Now().UTC()
	step.At = step.FinishedAt
	step.Status = "ok"
	if err != nil {
		step.Status = "failed"
		step.Error = err.Error()
	}
	return step, checks, err
}

func readRecoveryContract(root, bundle string) (recoveryContract, error) {
	var contract recoveryContract
	path := filepath.Join(bundle, "recovery.json")
	if err := validateRecoveryArtifact(root, bundle, path); err != nil {
		return contract, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return contract, err
	}
	if err := json.Unmarshal(body, &contract); err != nil {
		return contract, err
	}
	if contract.Version != 1 {
		return contract, fmt.Errorf("unsupported recovery contract version %d", contract.Version)
	}
	return contract, nil
}

func resolveSavedMigration(root, bundle string, saved *Migration) (*Migration, error) {
	if saved == nil {
		return nil, nil
	}
	migration := *saved
	if migration.Env.Source != "migration.env" {
		return nil, fmt.Errorf("invalid saved migration env path")
	}
	path := filepath.Join(bundle, "migration.env")
	if err := validateRecoveryArtifact(root, bundle, path); err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return nil, err
	}
	migration.Env.Source = relative
	if _, err := loadSelectedEnv(root, migration.Env); err != nil {
		return nil, err
	}
	return &migration, nil
}

func savedMigration(root, bundle string) (*Migration, error) {
	contract, err := readRecoveryContract(root, bundle)
	if err != nil {
		return nil, err
	}
	return resolveSavedMigration(root, bundle, contract.Migration)
}
