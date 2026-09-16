package deploy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Deployer struct {
	Root       string
	Out        io.Writer
	Err        io.Writer
	Runner     Runner
	resolver   hostResolver
	ConfigPath string
}

func NewDeployer(root string, out io.Writer, errOut io.Writer) Deployer {
	return Deployer{
		Root: root,
		Out:  out,
		Err:  errOut,
		Runner: Runner{
			Stdout: out,
			Stderr: errOut,
		},
	}
}

func (d Deployer) Deploy() error {
	plan, err := LoadPlan(d.Root, d.configPath())
	if err != nil {
		return err
	}
	if plan.Config.Project.ResourceID() == plan.Config.Project.Name {
		fmt.Fprintf(d.Out, "state v%d: using existing resources %s for %s/%s; release history preserved\n", CurrentStateVersion, plan.Config.Project.ResourceID(), plan.Config.Project.Name, plan.Config.Project.Environment)
	}
	state, err := LoadState(d.Root, plan.Config.Project)
	if err != nil {
		return err
	}
	if state.CurrentReleaseID != "" {
		if _, err := LoadRelease(d.Root, plan.Config.Project, state.CurrentReleaseID); err != nil {
			return err
		}
	}
	if err := d.checkLegacyHosts(plan); err != nil {
		return err
	}
	if err := d.ResolveAutoPorts(&plan); err != nil {
		return err
	}
	bundle, err := RenderBundle(d.Root, plan)
	if err != nil {
		return err
	}

	record := recordFromPlan(plan, bundle, state.CurrentReleaseID)
	if err := SaveRelease(d.Root, record); err != nil {
		return err
	}

	fmt.Fprintf(d.Out, "release: %s\n", plan.ReleaseID)
	fmt.Fprintln(d.Out, "building image")
	if err := d.BuildImages(plan, bundle); err != nil {
		record.Status = "failed"
		record.Apply = StepRecord{Status: "failed", Error: err.Error(), At: time.Now().UTC()}
		_ = SaveRelease(d.Root, record)
		return err
	}

	migrationRan := false
	if plan.Config.Migrations != nil {
		fmt.Fprintln(d.Out, "running migration")
		migrationRecord, err := d.RunMigrationRecord(plan, false)
		record.Migration = migrationRecord
		if err != nil {
			record.Status = "failed"
			_ = SaveRelease(d.Root, record)
			return err
		}
		migrationRan = true
		_ = SaveRelease(d.Root, record)
	}

	fmt.Fprintln(d.Out, "applying hosts")
	checks, err := d.ApplyRelease(plan, bundle)
	record.Checks = checks
	if err != nil {
		record.Status = "failed"
		record.Apply = StepRecord{Status: "failed", Error: err.Error(), At: time.Now().UTC()}
		_ = SaveRelease(d.Root, record)
		rollbackErrors := []string{}
		if state.CurrentReleaseID != "" {
			fmt.Fprintf(d.Err, "apply failed; re-applying previous release %s\n", state.CurrentReleaseID)
			previous, loadErr := LoadRelease(d.Root, plan.Config.Project, state.CurrentReleaseID)
			if loadErr != nil {
				rollbackErrors = append(rollbackErrors, loadErr.Error())
			} else if _, rollbackErr := d.ApplyRelease(planForRecord(plan, previous), bundleFromRecord(previous)); rollbackErr != nil {
				rollbackErrors = append(rollbackErrors, rollbackErr.Error())
			}
		}
		if migrationRan {
			fmt.Fprintln(d.Err, "apply failed after migration; running migration rollback")
			rollbackRecord, rollbackErr := d.RunMigrationRecord(plan, true)
			record.Rollback = rollbackRecord
			_ = SaveRelease(d.Root, record)
			if rollbackErr != nil {
				rollbackErrors = append(rollbackErrors, "migration rollback: "+rollbackErr.Error())
			}
		}
		if len(rollbackErrors) > 0 {
			return fmt.Errorf("%w; rollback also had errors: %s", err, strings.Join(rollbackErrors, "; "))
		}
		return err
	}

	now := time.Now().UTC()
	record.Status = "ok"
	record.Apply = StepRecord{Status: "ok", At: now}
	record.Checks = checks
	for i := range record.Hosts {
		record.Hosts[i].Status = "ok"
	}
	record.UpdatedAt = now
	if err := SaveRelease(d.Root, record); err != nil {
		return err
	}
	if err := SaveState(d.Root, State{
		ResourceID:        plan.Config.Project.ResourceID(),
		Project:           plan.Config.Project.Name,
		Environment:       plan.Config.Project.Environment,
		CurrentReleaseID:  plan.ReleaseID,
		PreviousReleaseID: state.CurrentReleaseID,
	}); err != nil {
		return err
	}

	fmt.Fprintf(d.Out, "deployed %s\n", plan.ReleaseID)
	d.CheckDomains(plan)
	return nil
}

func (d Deployer) BuildImages(plan Plan, bundle Bundle) error {
	envValues, err := loadConfigEnv(d.Root, plan.Config.Env)
	if err != nil {
		return err
	}
	for _, image := range bundle.Images {
		args := []string{"build", "-f", image.Build.Dockerfile}
		if image.Build.Target != "" {
			args = append(args, "--target", image.Build.Target)
		}
		if len(image.Build.Platforms) > 0 {
			args = append(args, "--platform", image.Build.Platforms[0])
		}
		for key, value := range image.Build.Args {
			args = append(args, "--build-arg", key+"="+RenderValue(value, plan, "", envValues))
		}
		for _, tag := range image.Tags {
			args = append(args, "-t", tag)
		}
		args = append(args, image.Build.Context)
		if err := d.Runner.Run(d.Root, "docker", args...); err != nil {
			return err
		}

		saveArgs := append([]string{"save", "-o", image.Tar}, image.Tags...)
		if err := d.Runner.Run(d.Root, "docker", saveArgs...); err != nil {
			return err
		}
	}
	return nil
}

func (d Deployer) RunMigration(plan Plan, rollback bool) error {
	_, err := d.RunMigrationRecord(plan, rollback)
	return err
}

func (d Deployer) RunMigrationRecord(plan Plan, rollback bool) (StepRecord, error) {
	migration := plan.Config.Migrations
	if migration == nil {
		return StepRecord{Status: "skipped", At: time.Now().UTC()}, nil
	}
	image := RenderTemplate(migration.Image, VarsForPlan(plan))
	command := migration.Command
	if rollback {
		command = migration.RollbackCommand
	}
	record := StepRecord{
		Status:      "running",
		StartedAt:   time.Now().UTC(),
		Command:     append([]string(nil), command...),
		Image:       image,
		EnvSource:   migration.Env.Source,
		RestorePlan: migration.RestorePlan,
	}
	err := d.runMigration(migration, image, rollback)
	record.FinishedAt = time.Now().UTC()
	record.At = record.FinishedAt
	if err != nil {
		record.Status = "failed"
		record.Error = err.Error()
		return record, err
	}
	record.Status = "ok"
	return record, nil
}

func (d Deployer) runMigration(migration *Migration, image string, rollback bool) error {
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

	args := []string{"run", "--rm"}
	if migration.Env.Source != "" {
		args = append(args, "--env-file", filepath.Join(d.Root, migration.Env.Source))
	}
	args = append(args, image)
	args = append(args, command...)
	ctx, cancel := optionalTimeoutContext(migration.TimeoutSeconds)
	defer cancel()
	return d.Runner.RunEnvContext(ctx, d.Root, nil, "docker", args...)
}

func (d Deployer) ApplyBundle(plan Plan, bundle Bundle) error {
	for _, host := range bundle.Hosts {
		fmt.Fprintf(d.Out, "host %s: upload\n", host.ID)
		if err := d.UploadHostBundle(host, bundle.Images); err != nil {
			return fmt.Errorf("host %s upload: %w", host.ID, err)
		}
		for _, image := range bundle.Images {
			fmt.Fprintf(d.Out, "host %s: docker load %s\n", host.ID, image.ID)
			if err := d.Remote(host.SSH, "docker load -i "+shellQuote(host.RemoteDir+"/images/"+filepath.Base(image.Tar))); err != nil {
				return fmt.Errorf("host %s docker load: %w", host.ID, err)
			}
		}
		if err := d.PullHostImages(plan, host); err != nil {
			return err
		}
		fmt.Fprintf(d.Out, "host %s: compose up\n", host.ID)
		if err := d.ComposeUp(plan, host, nil, false); err != nil {
			return fmt.Errorf("host %s compose up: %w", host.ID, err)
		}
	}
	return nil
}

func (d Deployer) ApplyRelease(plan Plan, bundle Bundle) ([]CheckRecord, error) {
	if !needsPhasedApply(plan) {
		if err := d.ApplyBundle(plan, bundle); err != nil {
			return nil, err
		}
		if err := d.ApplyRoutes(plan, bundle); err != nil {
			return nil, err
		}
		return d.RunChecks(plan, "smoke")
	}

	if err := d.UploadLoadPull(plan, bundle); err != nil {
		return nil, err
	}
	if err := d.ComposeUpPhase(plan, bundle, []string{"infra"}, true); err != nil {
		return nil, err
	}
	if err := d.ComposeUpPhase(plan, bundle, []string{"backend"}, true); err != nil {
		return nil, err
	}
	if err := d.ApplyRoutes(plan, bundle); err != nil {
		return nil, err
	}
	if err := d.RunHooks(plan, "after_backend_healthy"); err != nil {
		return nil, err
	}
	if err := d.RunHooks(plan, "after_services_healthy"); err != nil {
		return nil, err
	}
	if err := d.ComposeUpPhase(plan, bundle, nil, false); err != nil {
		return nil, err
	}
	if err := d.ApplyRoutes(plan, bundle); err != nil {
		return nil, err
	}
	return d.RunChecks(plan, "smoke")
}

func (d Deployer) UploadLoadPull(plan Plan, bundle Bundle) error {
	for _, host := range bundle.Hosts {
		fmt.Fprintf(d.Out, "host %s: upload\n", host.ID)
		if err := d.UploadHostBundle(host, bundle.Images); err != nil {
			return fmt.Errorf("host %s upload: %w", host.ID, err)
		}
		for _, image := range bundle.Images {
			fmt.Fprintf(d.Out, "host %s: docker load %s\n", host.ID, image.ID)
			if err := d.Remote(host.SSH, "docker load -i "+shellQuote(host.RemoteDir+"/images/"+filepath.Base(image.Tar))); err != nil {
				return fmt.Errorf("host %s docker load: %w", host.ID, err)
			}
		}
		if err := d.PullHostImages(plan, host); err != nil {
			return err
		}
	}
	return nil
}

func (d Deployer) ComposeUpPhase(plan Plan, bundle Bundle, phases []string, wait bool) error {
	for _, host := range bundle.Hosts {
		services := servicesForPhases(plan, host.ServiceIDs, phases)
		if phases != nil && len(services) == 0 {
			continue
		}
		fmt.Fprintf(d.Out, "host %s: compose up\n", host.ID)
		if err := d.ComposeUp(plan, host, services, wait); err != nil {
			return fmt.Errorf("host %s compose up: %w", host.ID, err)
		}
	}
	return nil
}

func (d Deployer) PullHostImages(plan Plan, host HostBundle) error {
	if len(host.PullServices) == 0 {
		return nil
	}
	fmt.Fprintf(d.Out, "host %s: checking images\n", host.ID)
	if err := d.Runner.SSHScript(d.Root, host.SSH, pullHostImagesScript(plan, host)); err != nil {
		return fmt.Errorf("host %s image pull: %w", host.ID, err)
	}
	return nil
}

// Inspect the resolved image locally: Compose's missing policy still pulls :latest.
func pullHostImagesScript(plan Plan, host HostBundle) string {
	compose := "docker compose -f compose.yml -p " + shellQuote(plan.Config.Project.ResourceID())
	script := "set -eu\ncd " + shellQuote(host.RemoteDir) + "\n"
	for _, serviceID := range host.PullServices {
		service := shellQuote(serviceID)
		pull := compose + " pull --policy always " + service + "\n"
		switch plan.Config.Services[serviceID].Pull {
		case "always":
			script += pull
		case "if_missing":
			script += "image=$(" + compose + " config --images " + service + ")\n" +
				"if docker image inspect \"$image\" >/dev/null 2>&1; then\n" +
				"  printf 'using cached image %s\\n' \"$image\"\n" +
				"else\n  " + pull + "fi\n"
		}
	}
	return script
}

func (d Deployer) ComposeUp(plan Plan, host HostBundle, services []string, wait bool) error {
	args := []string{"docker compose -f compose.yml -p " + shellQuote(plan.Config.Project.ResourceID()) + " up -d --pull never"}
	if wait {
		args[0] += " --wait"
	}
	if len(services) > 0 {
		args[0] += " " + shellJoin(services)
	}
	composeCmd := "cd " + shellQuote(host.RemoteDir) + " && " + args[0]
	return d.Remote(host.SSH, composeCmd)
}

func (d Deployer) RunHooks(plan Plan, phase string) error {
	for _, hook := range plan.Config.Hooks[phase] {
		if len(hook.Run) == 0 {
			continue
		}
		fmt.Fprintf(d.Out, "hook %s: %s\n", phase, hook.Name)
		env := map[string]string{}
		if hook.Env.Source != "" {
			values, err := LoadEnvFile(filepath.Join(d.Root, hook.Env.Source))
			if err != nil {
				return fmt.Errorf("hook %s env: %w", hook.Name, err)
			}
			env = values
		}
		ctx, cancel := optionalTimeoutContext(hook.TimeoutSeconds)
		err := d.Runner.RunEnvContext(ctx, d.Root, env, hook.Run[0], hook.Run[1:]...)
		cancel()
		if err != nil {
			return fmt.Errorf("hook %s: %w", hook.Name, err)
		}
	}
	return nil
}

func (d Deployer) RunChecks(plan Plan, group string) ([]CheckRecord, error) {
	checks := plan.Config.Checks[group]
	if len(checks) == 0 {
		return nil, nil
	}
	records := make([]CheckRecord, 0, len(checks))
	for _, check := range checks {
		record, err := d.RunCheck(plan, group, check)
		records = append(records, record)
		if err != nil {
			return records, err
		}
	}
	return records, nil
}

func (d Deployer) RunCheck(plan Plan, group string, check Check) (CheckRecord, error) {
	started := time.Now().UTC()
	record := CheckRecord{
		Group:     group,
		Name:      check.Name,
		Status:    "running",
		StartedAt: started,
	}
	if check.URL != "" {
		record.Type = "http"
		record.Target = check.URL
		record.ExpectedStatus = expectedStatus(check)
		record.FollowRedirects = checkFollowsRedirects(check)
		fmt.Fprintf(d.Out, "check %s: %s\n", check.Name, check.URL)
		err := d.runHTTPCheck(check, &record)
		finishCheckRecord(&record, err)
		return record, err
	}

	record.Type = "command"
	record.Target = strings.Join(check.Command, " ")
	record.EnvSource = check.Env.Source
	fmt.Fprintf(d.Out, "check %s: %s\n", check.Name, record.Target)
	err := d.runCommandCheck(check)
	finishCheckRecord(&record, err)
	return record, err
}

func (d Deployer) runHTTPCheck(check Check, record *CheckRecord) error {
	client := http.Client{Timeout: checkTimeout(check.TimeoutSeconds)}
	if !checkFollowsRedirects(check) {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	resp, err := client.Get(check.URL)
	if err != nil {
		return fmt.Errorf("check %s: %w", check.Name, err)
	}
	_ = resp.Body.Close()
	record.ActualStatus = resp.StatusCode
	expected := expectedStatus(check)
	if resp.StatusCode != expected {
		return fmt.Errorf("check %s: got status %d, want %d", check.Name, resp.StatusCode, expected)
	}
	return nil
}

func (d Deployer) runCommandCheck(check Check) error {
	env := map[string]string{}
	if check.Env.Source != "" {
		values, err := LoadEnvFile(filepath.Join(d.Root, check.Env.Source))
		if err != nil {
			return fmt.Errorf("check %s env: %w", check.Name, err)
		}
		env = values
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout(check.TimeoutSeconds))
	defer cancel()
	if err := d.Runner.RunEnvContext(ctx, d.Root, env, check.Command[0], check.Command[1:]...); err != nil {
		return fmt.Errorf("check %s: %w", check.Name, err)
	}
	return nil
}

func finishCheckRecord(record *CheckRecord, err error) {
	record.FinishedAt = time.Now().UTC()
	record.DurationMS = record.FinishedAt.Sub(record.StartedAt).Milliseconds()
	if err != nil {
		record.Status = "failed"
		record.Error = err.Error()
		return
	}
	record.Status = "ok"
}

func expectedStatus(check Check) int {
	if check.ExpectStatus != 0 {
		return check.ExpectStatus
	}
	return http.StatusOK
}

func checkFollowsRedirects(check Check) bool {
	if check.FollowRedirects == nil {
		return true
	}
	return *check.FollowRedirects
}

func checkTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func optionalTimeoutContext(seconds int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		return context.WithCancel(context.Background())
	}
	return context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
}

func (d Deployer) UploadHostBundle(host HostBundle, images []ImageBundle) error {
	dirs := shellJoin([]string{host.RemoteDir, host.RemoteDir + "/env", host.RemoteDir + "/images"})
	if err := d.Remote(host.SSH, "umask 077; mkdir -p "+dirs+" && chmod 0700 "+dirs); err != nil {
		return err
	}
	if err := d.Copy(host.Compose, host.SSH, host.RemoteDir+"/compose.yml"); err != nil {
		return err
	}
	if host.Routes != "" {
		if err := d.Copy(host.Routes, host.SSH, host.RemoteDir+"/routes.caddy"); err != nil {
			return err
		}
	}
	for _, envFile := range host.EnvFiles {
		if err := d.Copy(envFile, host.SSH, host.RemoteDir+"/env/"+filepath.Base(envFile)); err != nil {
			return err
		}
	}
	for _, image := range images {
		if err := d.Copy(image.Tar, host.SSH, host.RemoteDir+"/images/"+filepath.Base(image.Tar)); err != nil {
			return err
		}
	}
	files := []string{host.RemoteDir + "/compose.yml"}
	for _, envFile := range host.EnvFiles {
		files = append(files, host.RemoteDir+"/env/"+filepath.Base(envFile))
	}
	return d.Remote(host.SSH, "chmod 0600 "+shellJoin(files))
}

func (d Deployer) ApplyRoutes(plan Plan, bundle Bundle) error {
	for _, host := range bundle.Hosts {
		if host.Routes == "" {
			continue
		}
		fmt.Fprintf(d.Out, "host %s: caddy route\n", host.ID)
		routePath := plan.Config.Project.routePath()
		command := "mkdir -p /etc/pp/proxy/routes && install -m 0644 " +
			shellQuote(host.RemoteDir+"/routes.caddy") + " " + shellQuote(routePath) +
			" && caddy reload --config /etc/caddy/Caddyfile"
		if err := d.Remote(host.SSH, command); err != nil {
			return fmt.Errorf("host %s caddy reload: %w", host.ID, err)
		}
	}
	return nil
}

func (d Deployer) Remote(sshTarget string, command string) error {
	return d.Runner.Run(d.Root, "ssh", sshTarget, command)
}

func (d Deployer) Copy(local string, sshTarget string, remotePath string) error {
	return d.Runner.Run(d.Root, "scp", local, sshTarget+":"+remotePath)
}

func (d Deployer) Status() error {
	cfg, err := LoadConfig(d.Root, d.configPath())
	if err != nil {
		return err
	}
	state, err := LoadState(d.Root, cfg.Project)
	if err != nil {
		return err
	}
	if state.CurrentReleaseID == "" {
		fmt.Fprintln(d.Out, "no deployments recorded")
		return nil
	}
	record, err := LoadRelease(d.Root, cfg.Project, state.CurrentReleaseID)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "project: %s\n", state.Project)
	fmt.Fprintf(d.Out, "environment: %s\n", state.Environment)
	fmt.Fprintf(d.Out, "state_version: %d\nresources: %s\n", state.Version, state.ResourceID)
	fmt.Fprintf(d.Out, "current: %s\n", state.CurrentReleaseID)
	fmt.Fprintf(d.Out, "previous: %s\n", emptyDash(state.PreviousReleaseID))
	fmt.Fprintf(d.Out, "status: %s\n", record.Status)
	fmt.Fprintf(d.Out, "git_sha: %s\n", record.Git.SHA)
	fmt.Fprintf(d.Out, "dirty: %t\n", record.Git.Dirty)
	fmt.Fprintln(d.Out, "hosts:")
	for _, host := range record.Hosts {
		fmt.Fprintf(d.Out, "  - %s (%s): %s\n", host.ID, host.SSH, host.Status)
	}
	return nil
}

func (d Deployer) Down() error {
	plan, err := LoadPlan(d.Root, d.configPath())
	if err != nil {
		return err
	}
	if _, err := LoadState(d.Root, plan.Config.Project); err != nil {
		return err
	}
	if err := d.checkLegacyHosts(plan); err != nil {
		return err
	}

	for _, host := range plan.Hosts {
		if len(host.Services) == 0 {
			continue
		}
		fmt.Fprintf(d.Out, "host %s: down %s\n", host.ID, plan.Config.Project.DeploymentID())
		if err := d.Remote(host.SSH, downProjectContainersCommand(plan.Config.Project)); err != nil {
			return fmt.Errorf("host %s down: %w", host.ID, err)
		}
	}
	return nil
}

func (d Deployer) Rollback() error {
	plan, err := LoadPlan(d.Root, d.configPath())
	if err != nil {
		return err
	}
	state, err := LoadState(d.Root, plan.Config.Project)
	if err != nil {
		return err
	}
	if state.PreviousReleaseID == "" {
		return fmt.Errorf("no previous release recorded")
	}
	current, err := LoadRelease(d.Root, plan.Config.Project, state.CurrentReleaseID)
	if err != nil {
		return err
	}
	previous, err := LoadRelease(d.Root, plan.Config.Project, state.PreviousReleaseID)
	if err != nil {
		return err
	}

	if err := d.checkLegacyHosts(plan); err != nil {
		return err
	}
	if plan.Config.Migrations != nil && current.ReleaseID != "" {
		fmt.Fprintln(d.Out, "running DB rollback")
		image := plan.Config.Migrations.Image
		if len(current.ImageTags) > 0 {
			image = current.ImageTags[0]
		}
		if err := d.runMigration(plan.Config.Migrations, image, true); err != nil {
			return err
		}
	}

	bundle := bundleFromRecord(previous)
	fmt.Fprintf(d.Out, "rolling back to %s\n", previous.ReleaseID)
	if _, err := d.ApplyRelease(planForRecord(plan, previous), bundle); err != nil {
		return err
	}

	previous.Status = "ok"
	previous.Rollback = StepRecord{Status: "ok", At: time.Now().UTC()}
	if err := SaveRelease(d.Root, previous); err != nil {
		return err
	}
	return SaveState(d.Root, State{
		ResourceID:        previous.ResourceID,
		Project:           previous.Project,
		Environment:       previous.Environment,
		CurrentReleaseID:  previous.ReleaseID,
		PreviousReleaseID: current.ReleaseID,
	})
}

func recordFromPlan(plan Plan, bundle Bundle, previousReleaseID string) ReleaseRecord {
	hosts := make([]HostRecord, 0, len(bundle.Hosts))
	for _, host := range bundle.Hosts {
		hosts = append(hosts, HostRecord{
			ID:        host.ID,
			SSH:       host.SSH,
			Services:  host.ServiceIDs,
			RemoteDir: host.RemoteDir,
			Status:    "planned",
		})
	}
	images := make([]ImageRecord, 0, len(bundle.Images))
	for _, image := range bundle.Images {
		images = append(images, ImageRecord{
			ID:   image.ID,
			Tar:  image.Tar,
			Tags: image.Tags,
		})
	}
	now := time.Now().UTC()
	return ReleaseRecord{
		Version:           CurrentStateVersion,
		ResourceID:        plan.Config.Project.ResourceID(),
		Project:           plan.Config.Project.Name,
		Environment:       plan.Config.Project.Environment,
		ReleaseID:         plan.ReleaseID,
		PreviousReleaseID: previousReleaseID,
		Git:               plan.Git,
		ImageTags:         bundle.ImageTags,
		ImageTar:          bundle.ImageTar,
		Images:            images,
		BundlePath:        bundle.Root,
		RemoteBase:        plan.Config.Project.remoteDir(),
		Hosts:             hosts,
		Migration:         StepRecord{Status: "skipped"},
		Apply:             StepRecord{Status: "planned"},
		Status:            "planned",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func bundleFromRecord(record ReleaseRecord) Bundle {
	hosts := make([]HostBundle, 0, len(record.Hosts))
	for _, host := range record.Hosts {
		hostPath := filepath.Join(record.BundlePath, "hosts", host.ID)
		envFiles, _ := filepath.Glob(filepath.Join(hostPath, "env", "*.env"))
		hosts = append(hosts, HostBundle{
			ID:         host.ID,
			SSH:        host.SSH,
			Path:       hostPath,
			Compose:    filepath.Join(hostPath, "compose.yml"),
			Routes:     existingFile(filepath.Join(hostPath, "routes.caddy")),
			EnvFiles:   envFiles,
			RemoteDir:  host.RemoteDir,
			ServiceIDs: host.Services,
		})
	}
	images := []ImageBundle{}
	for _, image := range record.Images {
		images = append(images, ImageBundle{
			ID:   image.ID,
			Tar:  image.Tar,
			Tags: image.Tags,
		})
	}
	if len(images) == 0 && record.ImageTar != "" {
		images = append(images, ImageBundle{ID: "default", Tar: record.ImageTar, Tags: record.ImageTags})
	}
	return Bundle{
		Root:      record.BundlePath,
		ImageTar:  record.ImageTar,
		ImageTags: record.ImageTags,
		Images:    images,
		Hosts:     hosts,
	}
}

func planForRecord(base Plan, record ReleaseRecord) Plan {
	base.Config.Project.resourceID = record.ResourceID
	base.ReleaseID = record.ReleaseID
	base.Git = record.Git
	return base
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func existingFile(path string) string {
	if path != "" {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func needsPhasedApply(plan Plan) bool {
	if len(plan.Config.Hooks) > 0 {
		return true
	}
	for _, service := range plan.Config.Services {
		if service.Phase != "" {
			return true
		}
	}
	return false
}

func servicesForPhases(plan Plan, serviceIDs []string, phases []string) []string {
	if phases == nil {
		return serviceIDs
	}
	wanted := map[string]bool{}
	for _, phase := range phases {
		wanted[phase] = true
	}
	out := []string{}
	for _, serviceID := range serviceIDs {
		service := plan.Config.Services[serviceID]
		if wanted[service.Phase] {
			out = append(out, serviceID)
		}
	}
	return out
}

func shellJoin(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, shellQuote(value))
	}
	return strings.Join(quoted, " ")
}

func downProjectContainersCommand(project Project) string {
	projectFilter := shellQuote("label=pp.project=" + project.Name)
	environmentFilter := shellQuote("label=pp.environment=" + project.Environment)
	return "set -eu; ids=$(docker ps -aq --filter " + projectFilter + " --filter " + environmentFilter + "); " +
		"if [ -n \"$ids\" ]; then docker rm -f $ids; else echo " + shellQuote("no containers for deployment "+project.DeploymentID()) + "; fi"
}

func (d Deployer) configPath() string {
	if d.ConfigPath == "" {
		return "deploy.yml"
	}
	return d.ConfigPath
}
