package deploy

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ComposeFile struct {
	Services map[string]ComposeService `yaml:"services"`
	Volumes  map[string]ComposeVolume  `yaml:"volumes,omitempty"`
}

type ComposeService struct {
	Image       string            `yaml:"image"`
	Restart     string            `yaml:"restart,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Entrypoint  []string          `yaml:"entrypoint,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	EnvFile     []ComposeEnvFile  `yaml:"env_file,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Healthcheck *Healthcheck      `yaml:"healthcheck,omitempty"`
}

type Healthcheck struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval,omitempty"`
	Timeout  string   `yaml:"timeout,omitempty"`
	Retries  int      `yaml:"retries,omitempty"`
}

type ComposeEnvFile struct {
	Path   string `yaml:"path"`
	Format string `yaml:"format"`
}

type ComposeVolume struct {
	Driver   string `yaml:"driver,omitempty"`
	External bool   `yaml:"external,omitempty"`
}

type Bundle struct {
	Root      string
	ImageTar  string
	ImageTags []string
	Images    []ImageBundle
	Hosts     []HostBundle
}

type ImageBundle struct {
	ID    string
	Tar   string
	Tags  []string
	Build Build
}

type HostBundle struct {
	ID           string
	SSH          string
	Path         string
	Compose      string
	Routes       string
	EnvFiles     []string
	RemoteDir    string
	ServiceIDs   []string
	PullServices []string
}

func RenderBundle(root string, plan Plan) (Bundle, error) {
	vars := VarsForPlan(plan)
	envValues, err := loadConfigEnv(root, plan.Config.Env)
	if err != nil {
		return Bundle{}, err
	}
	bundleRoot := filepath.Join(plan.Config.Project.localDir(root), "releases", plan.ReleaseID)
	imagesDir := filepath.Join(bundleRoot, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return Bundle{}, fmt.Errorf("create image dir: %w", err)
	}
	if err := os.Chmod(bundleRoot, 0700); err != nil {
		return Bundle{}, fmt.Errorf("protect release directory: %w", err)
	}

	bundle := Bundle{
		Root: bundleRoot,
	}
	for _, image := range renderImageBundles(imagesDir, plan, envValues) {
		bundle.Images = append(bundle.Images, image)
	}
	if len(bundle.Images) > 0 {
		bundle.ImageTar = bundle.Images[0].Tar
		bundle.ImageTags = bundle.Images[0].Tags
	}

	for _, hostPlan := range plan.Hosts {
		hostDir := filepath.Join(bundleRoot, "hosts", hostPlan.ID)
		envDir := filepath.Join(hostDir, "env")
		if err := os.MkdirAll(envDir, 0700); err != nil {
			return Bundle{}, fmt.Errorf("create host bundle dir: %w", err)
		}

		compose := ComposeFile{
			Services: map[string]ComposeService{},
			Volumes:  map[string]ComposeVolume{},
		}
		envFiles := []string{}
		pullServices := []string{}

		for _, serviceID := range hostPlan.Services {
			service := plan.Config.Services[serviceID]
			envFile, err := renderServiceEnv(root, envDir, serviceID, service.Env)
			if err != nil {
				return Bundle{}, err
			}
			if envFile != "" {
				envFiles = append(envFiles, envFile)
			}

			composeService, err := renderComposeService(plan, hostPlan.ID, serviceID, service, vars, envValues, envFile)
			if err != nil {
				return Bundle{}, err
			}
			compose.Services[serviceID] = composeService
			if shouldPullService(service) {
				pullServices = append(pullServices, serviceID)
			}
			for _, mount := range service.Volumes {
				if mount.Name != "" {
					volume := plan.Config.Volumes[mount.Name]
					compose.Volumes[mount.Name] = ComposeVolume{
						Driver:   volume.Driver,
						External: volume.External,
					}
				}
			}
		}
		if len(compose.Volumes) == 0 {
			compose.Volumes = nil
		}

		composePath := filepath.Join(hostDir, "compose.yml")
		if err := writeComposeYAML(composePath, compose); err != nil {
			return Bundle{}, err
		}
		routesPath, err := renderCaddyRoutes(root, hostDir, plan, hostPlan.ID, envValues)
		if err != nil {
			return Bundle{}, err
		}

		bundle.Hosts = append(bundle.Hosts, HostBundle{
			ID:           hostPlan.ID,
			SSH:          hostPlan.SSH,
			Path:         hostDir,
			Compose:      composePath,
			Routes:       routesPath,
			EnvFiles:     envFiles,
			RemoteDir:    remoteReleaseDir(plan.Config.Project, plan.ReleaseID),
			ServiceIDs:   hostPlan.Services,
			PullServices: pullServices,
		})
	}

	return bundle, nil
}

func renderImageBundles(imagesDir string, plan Plan, envValues map[string]string) []ImageBundle {
	vars := VarsForPlan(plan)
	if len(plan.Config.Builds) == 0 {
		build := plan.Config.Build
		tags := RenderTemplates(build.Tags, vars)
		if len(tags) == 0 {
			tags = []string{fmt.Sprintf("%s:%s", plan.Config.Project.Name, plan.Git.SHA)}
		}
		return []ImageBundle{{
			ID:    "default",
			Tar:   filepath.Join(imagesDir, plan.Config.Project.Name+"-"+plan.ReleaseID+".tar"),
			Tags:  tags,
			Build: build,
		}}
	}
	out := []ImageBundle{}
	used := map[string]bool{}
	for _, item := range sortedMap(plan.Config.Services) {
		service := item.Value
		if service.Build == "" || used[service.Build] {
			continue
		}
		build := plan.Config.Builds[service.Build]
		tags := make([]string, 0, len(build.Tags))
		for _, tag := range build.Tags {
			tags = append(tags, RenderValue(tag, plan, "", envValues))
		}
		if len(tags) == 0 {
			tags = []string{fmt.Sprintf("%s-%s:%s", plan.Config.Project.Name, service.Build, plan.Git.SHA)}
		}
		out = append(out, ImageBundle{
			ID:    service.Build,
			Tar:   filepath.Join(imagesDir, plan.Config.Project.Name+"-"+service.Build+"-"+plan.ReleaseID+".tar"),
			Tags:  tags,
			Build: build,
		})
		used[service.Build] = true
	}
	return out
}

func loadConfigEnv(root string, env EnvSpec) (map[string]string, error) {
	if env.Source == "" {
		return map[string]string{}, nil
	}
	values, err := LoadEnvFile(filepath.Join(root, env.Source))
	if err != nil {
		return nil, fmt.Errorf("load config env: %w", err)
	}
	return values, nil
}

func renderCaddyRoutes(root string, hostDir string, plan Plan, hostID string, envValues map[string]string) (string, error) {
	var body strings.Builder
	for _, route := range plan.Config.Routes {
		if !routeBelongsToHost(plan.Config, route, hostID) {
			continue
		}
		target, err := routeTarget(plan, hostID, route)
		if err != nil {
			return "", err
		}
		body.WriteString(route.Host)
		body.WriteString(" {\n")
		body.WriteString("\treverse_proxy ")
		body.WriteString(target)
		body.WriteString("\n")
		body.WriteString("}\n\n")
	}
	for _, routeFile := range plan.Config.RouteFiles {
		if routeFile.Host != hostID {
			continue
		}
		source := filepath.Join(root, routeFile.Source)
		raw, err := os.ReadFile(source)
		if err != nil {
			return "", fmt.Errorf("read route file %s: %w", routeFile.Source, err)
		}
		body.WriteString(RenderValue(string(raw), plan, hostID, envValues))
		if !strings.HasSuffix(body.String(), "\n") {
			body.WriteByte('\n')
		}
	}
	if body.Len() == 0 {
		return "", nil
	}
	path := filepath.Join(hostDir, "routes.caddy")
	if err := os.WriteFile(path, []byte(body.String()), 0644); err != nil {
		return "", fmt.Errorf("write caddy routes: %w", err)
	}
	return path, nil
}

func routeTarget(plan Plan, hostID string, route Route) (string, error) {
	if route.Target != "" {
		return route.Target, nil
	}
	service, ok := plan.Config.Services[route.Service]
	if !ok {
		return "", fmt.Errorf("route %s references unknown service %s", route.Host, route.Service)
	}
	port, ok := selectRoutePort(service, route.TargetPort)
	if !ok {
		return "", fmt.Errorf("route %s cannot infer target for service %s", route.Host, route.Service)
	}
	published := port.Published.Value
	if port.Published.Auto {
		allocated, ok := plan.AutoPort(hostID, route.Service, port.Target)
		if !ok {
			return "", fmt.Errorf("route %s needs allocated port for %s:%d", route.Host, route.Service, port.Target)
		}
		published = allocated
	}
	hostIP := port.HostIP
	if hostIP == "" {
		hostIP = "127.0.0.1"
	}
	return "http://" + hostIP + ":" + strconv.Itoa(published), nil
}

func selectRoutePort(service Service, targetPort int) (Port, bool) {
	if targetPort == 0 {
		if len(service.Ports) != 1 {
			return Port{}, false
		}
		return service.Ports[0], true
	}
	for _, port := range service.Ports {
		if port.Target == targetPort {
			return port, true
		}
	}
	return Port{}, false
}

func routeBelongsToHost(cfg Config, route Route, hostID string) bool {
	if route.HostID != "" {
		return route.HostID == hostID
	}
	service, ok := cfg.Services[route.Service]
	return ok && contains(service.Hosts, hostID)
}

func renderComposeService(plan Plan, hostID string, serviceID string, service Service, vars RenderVars, envValues map[string]string, envFile string) (ComposeService, error) {
	labels := map[string]string{
		"pp.project":     plan.Config.Project.Name,
		"pp.environment": plan.Config.Project.Environment,
		"pp.release":     plan.ReleaseID,
		"pp.service":     serviceID,
		"pp.git_sha":     plan.Git.SHA,
	}

	out := ComposeService{
		Image:       RenderValue(service.Image, plan, hostID, envValues),
		Restart:     "unless-stopped",
		Command:     service.Command,
		Entrypoint:  service.Entrypoint,
		Ports:       renderPorts(plan, hostID, serviceID, service.Ports),
		Volumes:     renderVolumeMounts(service.Volumes),
		Labels:      labels,
		Environment: renderEnvironment(plan, hostID, envValues, service.Environment, service.CommandEnv),
	}
	if envFile != "" {
		out.EnvFile = []ComposeEnvFile{{Path: filepath.ToSlash(filepath.Join("env", filepath.Base(envFile))), Format: "raw"}}
	}
	if service.Health.HTTP != "" {
		healthURL := RenderValue(service.Health.HTTP, plan, hostID, envValues)
		parsed, err := url.Parse(healthURL)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || strings.ContainsFunc(healthURL, func(r rune) bool { return r <= ' ' || r == 127 }) {
			return ComposeService{}, fmt.Errorf("service %s health.http must render to an HTTP(S) URL without credentials, whitespace or control characters", serviceID)
		}
		out.Healthcheck = &Healthcheck{
			Test:     []string{"CMD", "wget", "-q", "--spider", "--", healthURL},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  healthRetries(service.Health.TimeoutSeconds),
		}
	} else if len(service.Health.Command) > 0 {
		command := make([]string, 0, len(service.Health.Command))
		for _, arg := range service.Health.Command {
			command = append(command, RenderValue(arg, plan, hostID, envValues))
		}
		out.Healthcheck = &Healthcheck{
			Test:     append([]string{"CMD"}, command...),
			Interval: "10s",
			Timeout:  "5s",
			Retries:  healthRetries(service.Health.TimeoutSeconds),
		}
	}
	return out, nil
}

func renderServiceEnv(root string, envDir string, serviceID string, env EnvSpec) (string, error) {
	if env.Source == "" {
		return "", nil
	}
	values, err := LoadEnvFile(filepath.Join(root, env.Source))
	if err != nil {
		return "", fmt.Errorf("load env for %s: %w", serviceID, err)
	}
	selected := map[string]bool{}
	for _, key := range env.Required {
		if _, ok := values[key]; !ok {
			return "", fmt.Errorf("env for %s is missing required key %s", serviceID, key)
		}
		selected[key] = true
	}
	if env.IncludeAll {
		for key := range values {
			selected[key] = true
		}
	}
	if len(selected) == 0 {
		return "", fmt.Errorf("env for %s must list required keys or explicitly set include_all: true", serviceID)
	}
	keys := make([]string, 0, len(selected))
	for key := range selected {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var body strings.Builder
	for _, key := range keys {
		body.WriteString(key)
		body.WriteByte('=')
		if strings.ContainsAny(key, "=\r\n") || strings.ContainsAny(values[key], "\r\n") {
			return "", fmt.Errorf("env for %s contains an unsupported multiline key or value", serviceID)
		}
		body.WriteString(values[key])
		body.WriteByte('\n')
	}

	path := filepath.Join(envDir, serviceID+".env")
	if err := os.WriteFile(path, []byte(body.String()), 0600); err != nil {
		return "", fmt.Errorf("write env file for %s: %w", serviceID, err)
	}
	return path, nil
}

func renderPorts(plan Plan, hostID string, serviceID string, ports []Port) []string {
	out := make([]string, 0, len(ports))
	for _, port := range ports {
		published := port.Published.Value
		if port.Published.Auto {
			allocated, ok := plan.AutoPort(hostID, serviceID, port.Target)
			if !ok {
				continue
			}
			published = allocated
		}
		value := strconv.Itoa(published) + ":" + strconv.Itoa(port.Target)
		if port.HostIP != "" {
			value = port.HostIP + ":" + value
		} else {
			value = "127.0.0.1:" + value
		}
		out = append(out, value)
	}
	return out
}

func renderVolumeMounts(mounts []VolumeMount) []string {
	out := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		source := mount.Name
		if mount.Source != "" {
			source = mount.Source
		}
		value := source + ":" + mount.Target
		if mount.ReadOnly {
			value += ":ro"
		}
		out = append(out, value)
	}
	return out
}

func renderEnvironment(plan Plan, hostID string, envValues map[string]string, maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, values := range maps {
		for key, value := range values {
			out[key] = RenderValue(value, plan, hostID, envValues)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shouldPullService(service Service) bool {
	return service.Pull == "if_missing" || service.Pull == "always"
}

func healthRetries(timeoutSeconds int) int {
	if timeoutSeconds <= 0 {
		return 6
	}
	retries := timeoutSeconds / 10
	if retries < 1 {
		return 1
	}
	return retries
}

// Compose performs another interpolation pass after pp. Escape only YAML values,
// not mapping keys or the raw env files that Compose reads separately.
func writeComposeYAML(path string, compose ComposeFile) error {
	var node yaml.Node
	if err := node.Encode(compose); err != nil {
		return err
	}
	var escape func(*yaml.Node)
	escape = func(n *yaml.Node) {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
			n.Value = strings.ReplaceAll(n.Value, "$", "$$")
		}
		for i, child := range n.Content {
			if n.Kind == yaml.MappingNode && i%2 == 0 {
				continue
			}
			escape(child)
		}
	}
	escape(&node)
	return writeYAML(path, &node)
}

func writeYAML(path string, value any) error {
	body, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := atomicWrite(path, body); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func remoteReleaseDir(project Project, releaseID string) string {
	return project.remoteDir() + "/releases/" + releaseID
}
