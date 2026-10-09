package deploy

import "fmt"

func hasLegacyBuild(cfg Config) bool {
	return cfg.Build.Context != "" || cfg.Build.Dockerfile != "" || len(cfg.Build.Tags) > 0 || len(cfg.Build.Args) > 0 || len(cfg.Build.Platforms) > 0 || cfg.Build.Target != ""
}

func buildTags(plan Plan, id string, build Build, values map[string]string) []string {
	tags := make([]string, 0, len(build.Tags))
	for _, tag := range build.Tags {
		tags = append(tags, RenderValue(tag, plan, "", values))
	}
	if len(tags) == 0 {
		name := plan.Config.Project.Name
		if id != "default" {
			name += "-" + id
		}
		tags = []string{fmt.Sprintf("%s:%s", name, plan.Git.SHA)}
	}
	return tags
}

func buildImageProblems(plan Plan, values map[string]string) []string {
	var problems []string
	for _, item := range sortedMap(plan.Config.Services) {
		service := item.Value
		id := service.Build
		build := plan.Config.Build
		bound := false
		if len(plan.Config.Builds) > 0 {
			if id != "" {
				build, bound = plan.Config.Builds[id]
			}
		}
		if len(plan.Config.Builds) == 0 && hasLegacyBuild(plan.Config) && (id == "default" || (id == "" && service.Pull == "")) {
			bound = true
			id = "default"
		}
		if !bound {
			continue
		}
		if service.Pull != "" && service.Pull != "never" {
			problems = append(problems, "services."+item.Key+" cannot pull a built image")
		}
		image := RenderValue(service.Image, plan, "", values)
		if !contains(buildTags(plan, id, build, values), image) {
			problems = append(problems, "services."+item.Key+".image must match a tag from build "+id)
		}
	}
	return problems
}
