package deploy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (d Deployer) trackAttempt(state *State, record *ReleaseRecord, action string) (func(*error), error) {
	state.Project = record.Project
	state.Environment = record.Environment
	state.ResourceID = record.ResourceID
	state.LatestAttempt = &AttemptRecord{Action: action, ReleaseID: record.ReleaseID, Step: StepRecord{Status: "running", StartedAt: time.Now().UTC()}}
	if err := SaveState(d.Root, *state); err != nil {
		return nil, err
	}
	return func(result *error) {
		step := &state.LatestAttempt.Step
		step.FinishedAt = time.Now().UTC()
		step.At = step.FinishedAt
		step.Status = "ok"
		if *result != nil {
			step.Status = "failed"
			step.Error = (*result).Error()
			if action == "deploy" {
				record.Status = "failed"
			}
		}
		*result = errors.Join(*result, SaveRelease(d.Root, *record), SaveState(d.Root, *state))
	}, nil
}

func (d Deployer) recordHostProgress(record *ReleaseRecord) func(string, string) error {
	return func(id, step string) error {
		for i := range record.Hosts {
			if record.Hosts[i].ID == id {
				record.Hosts[i].ApplyStep = step
				record.Hosts[i].Status = "applying"
				if step != "applying" {
					record.Hosts[i].Status = "applied"
				}
				return SaveRelease(d.Root, *record)
			}
		}
		return fmt.Errorf("host %s missing from deployment record", id)
	}
}

func (d Deployer) Status() error {
	plan, err := d.cleanupPlan()
	if err != nil {
		return err
	}
	project := plan.Config.Project
	state, err := LoadState(d.Root, project)
	if err != nil {
		return err
	}
	fmt.Fprintf(d.Out, "project: %s\nenvironment: %s\nresources: %s\n", project.Name, project.Environment, project.ResourceID())
	fmt.Fprintf(d.Out, "local current: %s\nlocal previous: %s\n", emptyDash(state.CurrentReleaseID), emptyDash(state.PreviousReleaseID))
	if state.LatestAttempt != nil {
		attempt := state.LatestAttempt
		fmt.Fprintf(d.Out, "latest attempt: %s %s (%s)\n", attempt.Action, attempt.ReleaseID, attempt.Step.Status)
		if attempt.Step.Error != "" {
			fmt.Fprintf(d.Out, "  error: %s\n", attempt.Step.Error)
		}
	}
	if state.CurrentReleaseID == "" && state.LatestAttempt == nil {
		fmt.Fprintln(d.Out, "no deployments recorded")
		return nil
	}
	targets := map[string]HostRecord{}
	seen := map[string]bool{}
	for _, id := range []string{state.CurrentReleaseID, attemptReleaseID(state)} {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		record, err := LoadRelease(d.Root, project, id)
		if err != nil {
			return err
		}
		fmt.Fprintf(d.Out, "local release %s: %s\n", id, record.Status)
		if record.Rollback.Status != "" {
			fmt.Fprintf(d.Out, "  recovery: %s\n", record.Rollback.Status)
		}
		for _, host := range record.Hosts {
			fmt.Fprintf(d.Out, "  %s: %s %s\n", host.ID, host.Status, host.ApplyStep)
			targets[host.SSH] = host
		}
	}
	var failures []error
	for _, target := range sortedMap(targets) {
		ctx, cancel := context.WithTimeout(d.Runner.context(), 15*time.Second)
		runner := d.Runner
		runner.Context = ctx
		host := target.Value
		command := "docker ps -a --filter " + shellQuote("label=pp.project="+project.Name) + " --filter " + shellQuote("label=pp.environment="+project.Environment) + " --format " + shellQuote(`{{.Label "pp.service"}}\t{{.Label "pp.release"}}\t{{.State}}\t{{.Status}}`)
		if err := validateSSHTarget(host.SSH); err != nil {
			cancel()
			return err
		}
		observed, err := runner.Output(d.Root, "ssh", host.SSH, command)
		cancel()
		fmt.Fprintf(d.Out, "observed host %s (%s):\n", host.ID, host.SSH)
		if err != nil {
			fmt.Fprintln(d.Out, "  unreachable or Docker inspection failed")
			failures = append(failures, fmt.Errorf("host %s inspection: %w", host.ID, err))
			continue
		}
		if observed == "" {
			fmt.Fprintln(d.Out, "  no owned containers")
			continue
		}
		fmt.Fprintf(d.Out, "  service / release / state / health: %s\n", observed)
		fmt.Fprintf(d.Out, "  expected local release: %s\n", emptyDash(state.CurrentReleaseID))
		matches := map[string]bool{}
		drift := false
		for _, line := range strings.Split(observed, "\n") {
			fields := strings.Split(line, "\t")
			if len(fields) != 4 {
				drift = true
				continue
			}
			matches[fields[0]] = true
			if fields[1] != state.CurrentReleaseID || fields[2] != "running" || strings.Contains(fields[3], "unhealthy") || strings.Contains(fields[3], "health: starting") {
				drift = true
			}
		}
		for _, service := range host.Services {
			if !matches[service] {
				drift = true
			}
		}
		if drift {
			fmt.Fprintln(d.Out, "  differs from local current release or readiness")
		}

	}
	return errors.Join(failures...)
}
func attemptReleaseID(state State) string {
	if state.LatestAttempt == nil {
		return ""
	}
	return state.LatestAttempt.ReleaseID
}
