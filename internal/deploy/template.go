package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type RenderVars struct{ Project, Environment, GitSHA, ShortSHA, Release string }

func VarsForPlan(plan Plan) RenderVars {
	return RenderVars{Project: plan.Config.Project.Name, Environment: plan.Config.Project.Environment, GitSHA: plan.Git.SHA, ShortSHA: plan.Git.ShortSHA, Release: plan.ReleaseID}
}
func metadataReplacements(vars RenderVars) []string {
	return []string{"${project}", vars.Project, "${environment}", vars.Environment, "${git_sha}", vars.GitSHA, "${short_sha}", vars.ShortSHA, "${release}", vars.Release}
}
func RenderTemplate(value string, vars RenderVars) string {
	return strings.NewReplacer(metadataReplacements(vars)...).Replace(value)
}
func RenderTemplates(values []string, vars RenderVars) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = RenderTemplate(value, vars)
	}
	return out
}

// Replace once, so dollar expressions inside an env value remain literal.
func RenderValue(value string, plan Plan, hostID string, env map[string]string) string {
	replacements := metadataReplacements(VarsForPlan(plan))
	for _, item := range sortedMap(env) {
		replacements = append(replacements, "${env."+item.Key+"}", item.Value)
	}
	if hostID == "" && len(plan.Hosts) == 1 {
		hostID = plan.Hosts[0].ID
	}
	for _, item := range sortedMap(plan.Config.Services) {
		if !contains(item.Value.Hosts, hostID) {
			continue
		}
		for _, port := range item.Value.Ports {
			published := port.Published.Value
			if port.Published.Auto {
				var ok bool
				published, ok = plan.AutoPort(hostID, item.Key, port.Target)
				if !ok {
					continue
				}
			}
			replacements = append(replacements, "${service."+item.Key+".port."+strconv.Itoa(port.Target)+"}", strconv.Itoa(published))
		}
	}
	// Callers rendering a fixture can provide assignments without a config service.
	for _, service := range sortedMap(plan.AutoPorts[hostID]) {
		for target, published := range service.Value {
			replacements = append(replacements, "${service."+service.Key+".port."+strconv.Itoa(target)+"}", strconv.Itoa(published))
		}
	}
	return strings.NewReplacer(replacements...).Replace(value)
}

var inputPlaceholder = regexp.MustCompile(`\$\{(env|service)\.[^}]+\}`)

func validateTemplateInput(value string, plan Plan, hostID string, values map[string]string) error {
	for _, placeholder := range inputPlaceholder.FindAllString(value, -1) {
		if RenderValue(placeholder, plan, hostID, values) == placeholder {
			return fmt.Errorf("unresolved pp placeholder %s", placeholder)
		}
	}
	leftover := inputPlaceholder.ReplaceAllString(value, "")
	if strings.Contains(leftover, "${env.") || strings.Contains(leftover, "${service.") {
		return fmt.Errorf("malformed pp placeholder")
	}
	return nil
}

func validatePlanTemplates(root string, plan Plan, pending bool) error {
	values, err := loadConfigEnv(root, plan.Config.Env)
	if err != nil {
		return err
	}
	if pending {
		for _, host := range plan.Hosts {
			for _, id := range host.Services {
				for _, port := range plan.Config.Services[id].Ports {
					if port.Published.Auto {
						plan.SetAutoPort(host.ID, id, port.Target, autoPortStart)
					}
				}
			}
		}
	}
	check := func(host string, inputs []string, env map[string]string) error {
		for _, value := range inputs {
			if err := validateTemplateInput(value, plan, host, env); err != nil {
				return err
			}
		}
		return nil
	}
	for _, host := range plan.Hosts {
		for _, id := range host.Services {
			svc := plan.Config.Services[id]
			inputs := []string{svc.Image, svc.Health.HTTP}
			inputs = append(inputs, svc.Command...)
			inputs = append(inputs, svc.Entrypoint...)
			inputs = append(inputs, svc.Health.Command...)
			for _, mapValue := range []map[string]string{svc.Environment, svc.CommandEnv} {
				for _, value := range mapValue {
					inputs = append(inputs, value)
				}
			}
			for _, mount := range svc.Volumes {
				inputs = append(inputs, mount.Source, mount.Target)
			}
			if err := check(host.ID, inputs, values); err != nil {
				return fmt.Errorf("service %s: %w", id, err)
			}
		}
	}
	builds := map[string]Build{"default": plan.Config.Build}
	if len(plan.Config.Builds) > 0 {
		builds = plan.Config.Builds
	}
	for _, item := range sortedMap(builds) {
		inputs := append([]string(nil), item.Value.Tags...)
		for _, value := range item.Value.Args {
			inputs = append(inputs, value)
		}
		if err := check("", inputs, values); err != nil {
			return fmt.Errorf("build %s: %w", item.Key, err)
		}
	}
	for _, file := range plan.Config.RouteFiles {
		body, err := os.ReadFile(filepath.Join(root, file.Source))
		if err != nil {
			return err
		}
		if err := check(file.Host, []string{string(body)}, values); err != nil {
			return fmt.Errorf("route template: %w", err)
		}
	}
	for _, route := range plan.Config.Routes {
		host := route.HostID
		if host == "" && len(plan.Config.Services[route.Service].Hosts) == 1 {
			host = plan.Config.Services[route.Service].Hosts[0]
		}
		if err := check(host, []string{route.Target}, values); err != nil {
			return err
		}
		if route.Target != "" {
			if err := validateCaddyTarget(RenderValue(route.Target, plan, host, values)); err != nil {
				return err
			}
		}
	}
	for _, hooks := range plan.Config.Hooks {
		for _, hook := range hooks {
			selected, err := loadSelectedEnv(root, hook.Env)
			if err != nil {
				return err
			}
			if err := check("", hook.Run, selected); err != nil {
				return fmt.Errorf("hook %s: %w", hook.Name, err)
			}
		}
	}
	for _, checks := range plan.Config.Checks {
		for _, checkValue := range checks {
			selected := values
			if len(checkValue.Command) > 0 {
				selected, err = loadSelectedEnv(root, checkValue.Env)
				if err != nil {
					return err
				}
			}
			inputs := append([]string{checkValue.URL}, checkValue.Command...)
			if err := check("", inputs, selected); err != nil {
				return fmt.Errorf("check %s: %w", checkValue.Name, err)
			}
			if checkValue.URL != "" {
				if err := validateRouteTarget(RenderValue(checkValue.URL, plan, "", selected)); err != nil {
					return err
				}
			}
		}
	}
	if migration := plan.Config.Migrations; migration != nil {
		selected, err := loadSelectedEnv(root, migration.Env)
		if err != nil {
			return err
		}
		inputs := append(append([]string(nil), migration.Command...), migration.RollbackCommand...)
		if err := check("", inputs, selected); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
		if err := check("", []string{migration.Image}, values); err != nil {
			return err
		}
	}
	return nil
}

func renderArgs(args []string, plan Plan, host string, values map[string]string) []string {
	out := make([]string, len(args))
	for i, arg := range args {
		out[i] = RenderValue(arg, plan, host, values)
	}
	return out
}
