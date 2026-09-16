package deploy

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version    int                `yaml:"version"`
	Project    Project            `yaml:"project"`
	Env        EnvSpec            `yaml:"env"`
	Build      Build              `yaml:"build"`
	Builds     map[string]Build   `yaml:"builds"`
	Hosts      map[string]Host    `yaml:"hosts"`
	Services   map[string]Service `yaml:"services"`
	Routes     []Route            `yaml:"routes"`
	RouteFiles []RouteFile        `yaml:"route_files"`
	Volumes    map[string]Volume  `yaml:"volumes"`
	Hooks      map[string][]Hook  `yaml:"hooks"`
	Migrations *Migration         `yaml:"migrations"`
	Checks     map[string][]Check `yaml:"checks"`
}

type Project struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
	resourceID  string
}

type Build struct {
	Context    string            `yaml:"context"`
	Dockerfile string            `yaml:"dockerfile"`
	Target     string            `yaml:"target"`
	Platforms  []string          `yaml:"platforms"`
	Tags       []string          `yaml:"tags"`
	Args       map[string]string `yaml:"args"`
}

type Host struct {
	SSH   string   `yaml:"ssh"`
	Roles []string `yaml:"roles"`
}

type Service struct {
	Image       string            `yaml:"image"`
	Build       string            `yaml:"build"`
	Pull        string            `yaml:"pull"`
	Phase       string            `yaml:"phase"`
	Hosts       []string          `yaml:"hosts"`
	Command     []string          `yaml:"command"`
	Entrypoint  []string          `yaml:"entrypoint"`
	Env         EnvSpec           `yaml:"env"`
	Environment map[string]string `yaml:"environment"`
	CommandEnv  map[string]string `yaml:"command_env"`
	Ports       []Port            `yaml:"ports"`
	Volumes     []VolumeMount     `yaml:"volumes"`
	Health      Health            `yaml:"health"`
}

type Route struct {
	Host       string `yaml:"host"`
	Service    string `yaml:"service"`
	Target     string `yaml:"target"`
	TargetPort int    `yaml:"target_port"`
	HostID     string `yaml:"host_id"`
}

type RouteFile struct {
	Source   string `yaml:"source"`
	Host     string `yaml:"host"`
	DestName string `yaml:"dest_name"`
}

type EnvSpec struct {
	Source     string   `yaml:"source"`
	Required   []string `yaml:"required"`
	IncludeAll bool     `yaml:"include_all,omitempty"`
}

type Port struct {
	HostIP    string        `yaml:"host_ip"`
	Published PublishedPort `yaml:"published"`
	Target    int           `yaml:"target"`
}

type PublishedPort struct {
	Value int
	Auto  bool
}

func (p *PublishedPort) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("published port must be a number or auto")
	}
	if value.Value == "auto" {
		p.Auto = true
		p.Value = 0
		return nil
	}
	var decoded int
	if err := value.Decode(&decoded); err != nil {
		return fmt.Errorf("published port must be a number or auto")
	}
	p.Value = decoded
	p.Auto = false
	return nil
}

func (p PublishedPort) MarshalYAML() (any, error) {
	if p.Auto {
		return "auto", nil
	}
	return p.Value, nil
}

func FixedPort(value int) PublishedPort {
	return PublishedPort{Value: value}
}

func AutoPort() PublishedPort {
	return PublishedPort{Auto: true}
}

type VolumeMount struct {
	Name     string `yaml:"name"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	Type     string `yaml:"type"`
	ReadOnly bool   `yaml:"read_only"`
}

type Health struct {
	HTTP           string   `yaml:"http"`
	Command        []string `yaml:"command"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
}

type Volume struct {
	Driver   string `yaml:"driver"`
	External bool   `yaml:"external"`
}

type Migration struct {
	Image           string   `yaml:"image"`
	Command         []string `yaml:"command"`
	RollbackCommand []string `yaml:"rollback_command"`
	RestorePlan     string   `yaml:"restore_plan"`
	Env             EnvSpec  `yaml:"env"`
	Run             string   `yaml:"run"`
	TimeoutSeconds  int      `yaml:"timeout_seconds"`
}

type Hook struct {
	Name           string   `yaml:"name"`
	Run            []string `yaml:"run"`
	Env            EnvSpec  `yaml:"env"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
}

type Check struct {
	Name            string   `yaml:"name"`
	URL             string   `yaml:"url"`
	ExpectStatus    int      `yaml:"expect_status"`
	FollowRedirects *bool    `yaml:"follow_redirects"`
	Command         []string `yaml:"command"`
	Env             EnvSpec  `yaml:"env"`
	TimeoutSeconds  int      `yaml:"timeout_seconds"`
}

type ValidationError struct {
	Problems []string
}

func (e ValidationError) Error() string {
	out := ""
	for _, problem := range e.Problems {
		out += fmt.Sprintf("- %s\n", problem)
	}
	return out
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
var sshTargetPattern = regexp.MustCompile(`^([a-zA-Z0-9_][a-zA-Z0-9_.-]*@)?[a-zA-Z0-9_][a-zA-Z0-9_.-]*$`)

func validateSSHTarget(target string) error {
	if !sshTargetPattern.MatchString(target) {
		return fmt.Errorf("SSH target must be a hostname or user@hostname, without options, whitespace or shell syntax; use SSH config for ports and jump hosts")
	}
	return nil
}

var hookPhasePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9_-]*[a-z0-9])?$`)

func LoadConfig(root string, configPath string) (Config, error) {
	fullPath := filepath.Join(root, configPath)
	body, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("missing %s in %s", configPath, root)
		}
		return Config{}, fmt.Errorf("read %s: %w", configPath, err)
	}

	cfg := Config{Version: CurrentConfigVersion}
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", configPath, err)
	}

	if err := ValidateConfig(root, cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func ValidateConfig(root string, cfg Config) error {
	var problems []string

	if cfg.Version != CurrentConfigVersion {
		problems = append(problems, fmt.Sprintf("unsupported config version %d; this pp supports version %d (upgrade pp for newer configs)", cfg.Version, CurrentConfigVersion))
	}
	validateSlug(&problems, "project.name", cfg.Project.Name)
	validateSlug(&problems, "project.environment", cfg.Project.Environment)
	validateEnv(&problems, root, "env", cfg.Env)
	if len(cfg.Builds) == 0 {
		require(&problems, "build.context", cfg.Build.Context)
		require(&problems, "build.dockerfile", cfg.Build.Dockerfile)
	} else {
		for _, item := range sortedMap(cfg.Builds) {
			buildID := item.Key
			validateSlug(&problems, "builds."+buildID, buildID)
			validateBuild(&problems, "builds."+buildID, item.Value)
		}
	}

	if len(cfg.Hosts) == 0 {
		problems = append(problems, "hosts must contain at least one host")
	}
	if len(cfg.Services) == 0 {
		problems = append(problems, "services must contain at least one service")
	}

	for _, item := range sortedMap(cfg.Hosts) {
		hostID := item.Key
		host := item.Value
		validateSlug(&problems, "hosts."+hostID, hostID)
		require(&problems, "hosts."+hostID+".ssh", host.SSH)
		if err := validateSSHTarget(host.SSH); err != nil {
			problems = append(problems, "hosts."+hostID+".ssh: "+err.Error())
		}
	}

	hostPorts := map[string]map[int]string{}
	for _, item := range sortedMap(cfg.Services) {
		serviceID := item.Key
		service := item.Value
		validateSlug(&problems, "services."+serviceID, serviceID)
		require(&problems, "services."+serviceID+".image", service.Image)
		if service.Build != "" {
			if _, ok := cfg.Builds[service.Build]; len(cfg.Builds) > 0 && !ok {
				problems = append(problems, "services."+serviceID+".build references unknown build "+service.Build)
			}
		}
		if service.Pull != "" && service.Pull != "if_missing" && service.Pull != "always" && service.Pull != "never" {
			problems = append(problems, "services."+serviceID+".pull must be if_missing, always, or never")
		}
		if service.Phase != "" {
			validateSlug(&problems, "services."+serviceID+".phase", service.Phase)
		}
		if len(service.Hosts) == 0 {
			problems = append(problems, "services."+serviceID+".hosts must contain at least one host")
		}
		for _, hostID := range service.Hosts {
			if _, ok := cfg.Hosts[hostID]; !ok {
				problems = append(problems, "services."+serviceID+".hosts references unknown host "+hostID)
				continue
			}
			if _, ok := hostPorts[hostID]; !ok {
				hostPorts[hostID] = map[int]string{}
			}
			for _, port := range service.Ports {
				if (!port.Published.Auto && port.Published.Value <= 0) || port.Target <= 0 {
					problems = append(problems, "services."+serviceID+".ports must use positive published and target ports, or published: auto")
					continue
				}
				if port.Published.Auto {
					continue
				}
				if owner, exists := hostPorts[hostID][port.Published.Value]; exists {
					problems = append(problems, fmt.Sprintf("host %s published port %d conflicts between services %s and %s", hostID, port.Published.Value, owner, serviceID))
				}
				hostPorts[hostID][port.Published.Value] = serviceID
			}
		}
		validateEnv(&problems, root, "services."+serviceID+".env", service.Env)
		if service.Env.Source != "" && len(service.Env.Required) == 0 && !service.Env.IncludeAll {
			problems = append(problems, "services."+serviceID+".env.required must list injected keys, or set include_all: true explicitly")
		}
		validateHealth(&problems, "services."+serviceID+".health", service.Health, len(service.Ports) > 0)
		for _, mount := range service.Volumes {
			if mount.Target == "" {
				problems = append(problems, "services."+serviceID+".volumes.target is required")
				continue
			}
			if mount.Name == "" && mount.Source == "" {
				problems = append(problems, "services."+serviceID+".volumes.name or source is required")
				continue
			}
			if mount.Name != "" {
				if _, ok := cfg.Volumes[mount.Name]; !ok {
					problems = append(problems, "services."+serviceID+".volumes references unknown volume "+mount.Name)
				}
			}
			if mount.Type != "" && mount.Type != "bind" && mount.Type != "volume" {
				problems = append(problems, "services."+serviceID+".volumes.type must be bind or volume")
			}
		}
	}

	for i, route := range cfg.Routes {
		if err := validateCaddyHost(route.Host); err != nil {
			problems = append(problems, fmt.Sprintf("routes[%d].host: %s", i, err))
		}
		field := fmt.Sprintf("routes[%d]", i)
		require(&problems, field+".host", route.Host)
		require(&problems, field+".service", route.Service)
		if _, ok := cfg.Services[route.Service]; route.Service != "" && !ok {
			problems = append(problems, field+".service references unknown service "+route.Service)
		}
		if route.Target != "" {
			if err := validateCaddyTarget(route.Target); err != nil {
				problems = append(problems, field+".target "+err.Error())
			}
		} else {
			service, ok := cfg.Services[route.Service]
			if !ok || !routeCanDeriveTarget(service, route.TargetPort) {
				problems = append(problems, field+".target is required unless service has exactly one matching port")
			}
		}
		if route.HostID != "" {
			if _, ok := cfg.Hosts[route.HostID]; !ok {
				problems = append(problems, field+".host_id references unknown host "+route.HostID)
			}
		}
	}

	for volumeID := range cfg.Volumes {
		validateSlug(&problems, "volumes."+volumeID, volumeID)
	}

	for i, routeFile := range cfg.RouteFiles {
		field := fmt.Sprintf("route_files[%d]", i)
		require(&problems, field+".source", routeFile.Source)
		if routeFile.Source != "" {
			if _, err := os.Stat(filepath.Join(root, routeFile.Source)); err != nil {
				problems = append(problems, field+".source "+err.Error())
			}
		}
		require(&problems, field+".host", routeFile.Host)
		if routeFile.Host != "" {
			if _, ok := cfg.Hosts[routeFile.Host]; !ok {
				problems = append(problems, field+".host references unknown host "+routeFile.Host)
			}
		}
	}

	if cfg.Migrations != nil {
		validateMigration(&problems, root, *cfg.Migrations)
	}
	for phase, hooks := range cfg.Hooks {
		validateHookPhase(&problems, "hooks."+phase, phase)
		for i, hook := range hooks {
			validateHook(&problems, root, fmt.Sprintf("hooks.%s[%d]", phase, i), hook)
		}
	}
	for group, checks := range cfg.Checks {
		validateHookPhase(&problems, "checks."+group, group)
		for i, check := range checks {
			validateCheck(&problems, root, fmt.Sprintf("checks.%s[%d]", group, i), check)
		}
	}

	if len(problems) > 0 {
		return ValidationError{Problems: problems}
	}
	return nil
}

func validateBuild(problems *[]string, field string, build Build) {
	require(problems, field+".context", build.Context)
	require(problems, field+".dockerfile", build.Dockerfile)
	for key := range build.Args {
		if strings.TrimSpace(key) == "" {
			*problems = append(*problems, field+".args has empty key")
		}
	}
}

func validateMigration(problems *[]string, root string, migration Migration) {
	require(problems, "migrations.image", migration.Image)
	if len(migration.Command) == 0 {
		*problems = append(*problems, "migrations.command must contain at least one argument")
	}
	if len(migration.RollbackCommand) == 0 && migration.RestorePlan == "" {
		*problems = append(*problems, "migrations.rollback_command or migrations.restore_plan is required")
	}
	if migration.Run != "" && migration.Run != "before_services" {
		*problems = append(*problems, "migrations.run must be before_services when set")
	}
	validateEnv(problems, root, "migrations.env", migration.Env)
}

func validateHook(problems *[]string, root string, field string, hook Hook) {
	if strings.TrimSpace(hook.Name) == "" {
		*problems = append(*problems, field+".name is required")
	}
	if len(hook.Run) == 0 {
		*problems = append(*problems, field+".run must contain at least one argument")
	}
	validateEnv(problems, root, field+".env", hook.Env)
}

func validateCheck(problems *[]string, root string, field string, check Check) {
	if strings.TrimSpace(check.Name) == "" {
		*problems = append(*problems, field+".name is required")
	}
	hasURL := strings.TrimSpace(check.URL) != ""
	hasCommand := len(check.Command) > 0
	if hasURL == hasCommand {
		*problems = append(*problems, field+" must declare exactly one of url or command")
	}
	if hasURL {
		if err := validateRouteTarget(check.URL); err != nil {
			*problems = append(*problems, field+".url "+err.Error())
		}
	}
	if hasCommand {
		validateEnv(problems, root, field+".env", check.Env)
	}
}

func validateEnv(problems *[]string, root string, field string, env EnvSpec) {
	if env.Source == "" && len(env.Required) == 0 {
		return
	}
	if env.Source == "" {
		*problems = append(*problems, field+".source is required when required env names are declared")
		return
	}
	values, err := LoadEnvFile(filepath.Join(root, env.Source))
	if err != nil {
		*problems = append(*problems, field+".source "+err.Error())
		return
	}
	for _, name := range env.Required {
		if _, ok := values[name]; !ok {
			*problems = append(*problems, field+".required missing "+name+" in "+env.Source)
		}
	}
}

func validateHealth(problems *[]string, field string, health Health, externallyReachable bool) {
	if !externallyReachable {
		return
	}
	if health.HTTP == "" && len(health.Command) == 0 {
		*problems = append(*problems, field+" is required for services with published ports")
	}
}

func validateRouteTarget(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("must include host")
	}
	return nil
}

func validateCaddyHost(host string) error {
	name := strings.TrimSuffix(strings.TrimPrefix(host, "*."), ".")
	if name == "" || len(name) > 253 {
		return fmt.Errorf("must be a DNS name or a leading wildcard DNS name")
	}
	for _, label := range strings.Split(name, ".") {
		if len(label) > 63 || !slugPattern.MatchString(strings.ToLower(label)) {
			return fmt.Errorf("must be a DNS name or a leading wildcard DNS name")
		}
	}
	return nil
}

func validateCaddyTarget(target string) error {
	if err := validateRouteTarget(target); err != nil {
		return err
	}
	parsed, _ := url.Parse(target)
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(target, "{}\\\"'` \t\r\n") || strings.ContainsFunc(target, func(r rune) bool { return r < 32 || r == 127 }) {
		return fmt.Errorf("must be an HTTP(S) origin without credentials, path, query, fragment or Caddy syntax")
	}
	return nil
}

func routeCanDeriveTarget(service Service, targetPort int) bool {
	if len(service.Ports) == 0 {
		return false
	}
	if targetPort == 0 {
		return len(service.Ports) == 1
	}
	for _, port := range service.Ports {
		if port.Target == targetPort {
			return true
		}
	}
	return false
}

func require(problems *[]string, field string, value string) {
	if value == "" {
		*problems = append(*problems, field+" is required")
	}
}

func validateSlug(problems *[]string, field string, value string) {
	if value == "" {
		*problems = append(*problems, field+" is required")
		return
	}
	if !slugPattern.MatchString(value) {
		*problems = append(*problems, field+" must be a lowercase DNS-safe slug")
	}
}

func validateHookPhase(problems *[]string, field string, value string) {
	if !hookPhasePattern.MatchString(value) {
		*problems = append(*problems, field+" must be lowercase and contain only letters, numbers, hyphens, or underscores")
	}
}

func sortedMap[T any](items map[string]T) []struct {
	Key   string
	Value T
} {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]struct {
		Key   string
		Value T
	}, 0, len(keys))
	for _, key := range keys {
		out = append(out, struct {
			Key   string
			Value T
		}{Key: key, Value: items[key]})
	}
	return out
}
