package deploy

import (
	"fmt"
	"io"
	"sort"
	"time"
)

type Plan struct {
	Config    Config
	Git       GitMetadata
	ReleaseID string
	Hosts     []HostPlan
	AutoPorts AutoPortAssignments
}

type AutoPortAssignments map[string]map[string]map[int]int

type HostPlan struct {
	ID       string
	SSH      string
	Services []string
}

func LoadPlan(root string, configPath string) (Plan, error) {
	cfg, err := LoadConfig(root, configPath)
	if err != nil {
		return Plan{}, err
	}
	state, err := LoadState(root, cfg.Project)
	if err != nil {
		return Plan{}, err
	}
	cfg.Project.resourceID = state.ResourceID
	git, err := ReadGitMetadata(root)
	if err != nil {
		return Plan{}, fmt.Errorf("read git metadata: %w", err)
	}

	return BuildPlan(cfg, git, time.Now()), nil
}

func BuildPlan(cfg Config, git GitMetadata, now time.Time) Plan {
	hostPlans := make([]HostPlan, 0, len(cfg.Hosts))
	for _, hostItem := range sortedMap(cfg.Hosts) {
		hostID := hostItem.Key
		host := hostItem.Value
		services := make([]string, 0)
		for _, serviceItem := range sortedMap(cfg.Services) {
			serviceID := serviceItem.Key
			service := serviceItem.Value
			if contains(service.Hosts, hostID) {
				services = append(services, serviceID)
			}
		}
		hostPlans = append(hostPlans, HostPlan{
			ID:       hostID,
			SSH:      host.SSH,
			Services: services,
		})
	}

	return Plan{
		Config:    cfg,
		Git:       git,
		ReleaseID: ReleaseID(now, git.ShortSHA),
		Hosts:     hostPlans,
		AutoPorts: AutoPortAssignments{},
	}
}

func (p *Plan) SetAutoPort(hostID string, serviceID string, target int, published int) {
	if p.AutoPorts == nil {
		p.AutoPorts = AutoPortAssignments{}
	}
	if _, ok := p.AutoPorts[hostID]; !ok {
		p.AutoPorts[hostID] = map[string]map[int]int{}
	}
	if _, ok := p.AutoPorts[hostID][serviceID]; !ok {
		p.AutoPorts[hostID][serviceID] = map[int]int{}
	}
	p.AutoPorts[hostID][serviceID][target] = published
}

func (p Plan) AutoPort(hostID string, serviceID string, target int) (int, bool) {
	if p.AutoPorts == nil {
		return 0, false
	}
	services, ok := p.AutoPorts[hostID]
	if !ok {
		return 0, false
	}
	targets, ok := services[serviceID]
	if !ok {
		return 0, false
	}
	published, ok := targets[target]
	return published, ok
}

func PrintPlan(w io.Writer, plan Plan) {
	fmt.Fprintf(w, "project: %s\n", plan.Config.Project.Name)
	fmt.Fprintf(w, "environment: %s\n", plan.Config.Project.Environment)
	fmt.Fprintf(w, "deployment: %s\n", plan.Config.Project.DeploymentID())
	fmt.Fprintf(w, "resources: %s\n", plan.Config.Project.ResourceID())
	fmt.Fprintf(w, "release: %s\n", plan.ReleaseID)
	fmt.Fprintf(w, "git_sha: %s\n", plan.Git.SHA)
	fmt.Fprintf(w, "dirty: %t\n", plan.Git.Dirty)
	if plan.Git.Dirty {
		fmt.Fprintf(w, "worktree_diff_digest: %s\n", plan.Git.DiffDigest)
	}
	if plan.Config.Env.Source != "" {
		fmt.Fprintf(w, "env_source: %s\n", plan.Config.Env.Source)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "hosts:")
	for _, host := range plan.Hosts {
		fmt.Fprintf(w, "  - %s (%s)\n", host.ID, host.SSH)
		if len(host.Services) == 0 {
			fmt.Fprintln(w, "    services: []")
			continue
		}
		fmt.Fprintf(w, "    services: %s\n", joinComma(host.Services))
	}
	if plan.Config.Migrations != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "migration:")
		fmt.Fprintf(w, "  image: %s\n", plan.Config.Migrations.Image)
		if plan.Config.Migrations.Env.Source != "" {
			fmt.Fprintf(w, "  env_source: %s\n", plan.Config.Migrations.Env.Source)
		}
		fmt.Fprintf(w, "  rollback: %t\n", len(plan.Config.Migrations.RollbackCommand) > 0 || plan.Config.Migrations.RestorePlan != "")
	}
	if len(plan.Config.Checks) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "checks:")
		for _, group := range sortedMap(plan.Config.Checks) {
			fmt.Fprintf(w, "  %s: %d\n", group.Key, len(group.Value))
		}
	}
}

func contains(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}

func joinComma(items []string) string {
	copied := append([]string(nil), items...)
	sort.Strings(copied)
	out := ""
	for i, item := range copied {
		if i > 0 {
			out += ", "
		}
		out += item
	}
	return out
}
