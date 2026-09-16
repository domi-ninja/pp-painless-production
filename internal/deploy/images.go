package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type ManagedImage struct {
	Source    bool   `json:"source,omitempty"`
	Built     bool   `json:"built,omitempty"`
	Reference string `json:"reference"`
	ID        string `json:"id"`
}

type imageManifest struct {
	Version int            `json:"version"`
	Images  []ManagedImage `json:"images"`
}

var imageIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func readImageManifest(path string) (imageManifest, error) {
	m := imageManifest{Version: 1}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return m, err
	}
	if m.Version != 1 {
		return m, fmt.Errorf("unsupported image manifest version %d", m.Version)
	}
	for _, image := range m.Images {
		if !imageIDPattern.MatchString(image.ID) || image.Reference == "" || strings.HasPrefix(image.Reference, "-") || strings.ContainsAny(image.Reference, " \t\r\n") {
			return m, fmt.Errorf("invalid managed image record")
		}
	}
	return m, nil
}

func managedImagePrefix(project Project) string {
	return "pp.local/" + project.Name + "/" + project.Environment + "/"
}

func (d Deployer) recordBuiltImages(plan Plan, bundle Bundle, image ImageBundle) error {
	path := filepath.Join(bundle.Root, "images.json")
	m, err := readImageManifest(path)
	if err != nil {
		return err
	}
	for _, tag := range image.Tags {
		id, err := d.Runner.Output(d.Root, "docker", "image", "inspect", "--format", "{{.Id}}", tag)
		if err != nil {
			return err
		}
		if !imageIDPattern.MatchString(id) {
			return fmt.Errorf("invalid Docker image ID for %s", tag)
		}
		m.Images = append(m.Images, ManagedImage{Reference: tag, ID: id, Built: true})
	}
	return writeJSON(path, m)
}

// Match the rendered image names, not merely build IDs: legacy single-build
// configs often omit services.build entirely.
func imagesForHost(host HostBundle, images []ImageBundle) ([]ImageBundle, error) {
	body, err := os.ReadFile(host.Compose)
	if err != nil {
		return nil, err
	}
	var compose ComposeFile
	if err := yaml.Unmarshal(body, &compose); err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, service := range compose.Services {
		wanted[service.Image] = true
	}
	// Pinned rollback bundles no longer contain the original build tags.
	pins, err := readImageManifest(filepath.Join(host.Path, "images.json"))
	if err != nil {
		return nil, err
	}
	local, err := readImageManifest(filepath.Join(filepath.Dir(filepath.Dir(host.Path)), "images.json"))
	if err != nil {
		return nil, err
	}
	for _, pin := range pins.Images {
		for _, built := range local.Images {
			if pin.ID == built.ID {
				wanted[built.Reference] = true
			}
		}
	}
	var selected []ImageBundle
	for _, image := range images {
		for _, tag := range image.Tags {
			if wanted[tag] {
				selected = append(selected, image)
				break
			}
		}
	}
	return selected, nil
}

// Rewrite only image nodes, preserving literal-dollar escaping elsewhere.
// The aliases are unique per release, while the manifest records exact IDs.
func (d Deployer) pinHostImages(plan Plan, host HostBundle) error {
	path := filepath.Join(host.Path, "images.json")
	manifest, err := readImageManifest(path)
	if err != nil {
		return err
	}
	local, err := readImageManifest(filepath.Join(filepath.Dir(filepath.Dir(host.Path)), "images.json"))
	if err != nil {
		return err
	}
	body, err := os.ReadFile(host.Compose)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(body, &document); err != nil {
		return err
	}
	services := yamlMapValue(document.Content[0], "services")
	if services == nil {
		return fmt.Errorf("missing compose services")
	}
	updated := false
	for i := 0; i < len(services.Content); i += 2 {
		serviceID := services.Content[i].Value
		image := yamlMapValue(services.Content[i+1], "image")
		if image == nil {
			return fmt.Errorf("service %s has no image", serviceID)
		}
		for _, built := range local.Images {
			if built.Reference == image.Value {
				manifest.Images = append(manifest.Images, built)
			}
		}
		alias := managedImagePrefix(plan.Config.Project) + serviceID + ":" + strings.ToLower(plan.ReleaseID)
		if image.Value == alias {
			// Rollback must use the recorded immutable image, not a mutable tag.
			var id string
			for _, pin := range manifest.Images {
				if pin.Reference == alias {
					id = pin.ID
				}
			}
			if id == "" {
				return fmt.Errorf("missing image lock for %s", alias)
			}
			if err := d.Remote(host.SSH, "docker image inspect "+shellQuote(id)+" >/dev/null && docker tag "+shellQuote(id)+" "+shellQuote(alias)); err != nil {
				return err
			}
			continue
		}
		updated = true
		id, err := d.Runner.Output(d.Root, "ssh", host.SSH, "docker image inspect --format '{{.Id}}' "+shellQuote(image.Value))
		if err != nil {
			return err
		}
		if !imageIDPattern.MatchString(id) {
			return fmt.Errorf("invalid image ID on %s for %s", host.SSH, serviceID)
		}
		manifest.Images = append(manifest.Images, ManagedImage{Reference: alias, ID: id})
		if err := writeJSON(path, manifest); err != nil {
			return err
		}
		if err := d.Remote(host.SSH, "docker tag "+shellQuote(id)+" "+shellQuote(alias)); err != nil {
			return err
		}
		if err := d.finishSourceImage(plan, host, serviceID, image.Value, id, &manifest); err != nil {
			return err
		}
		image.Value = alias
	}
	if updated {
		var sources []ManagedImage
		for _, image := range manifest.Images {
			if image.Source {
				sources = append(sources, image)
			}
		}
		if len(sources) > 0 {
			if err := d.Remote(host.SSH, imageRemovalScript(sources)); err != nil {
				return err
			}
		}
	}
	body, err = yaml.Marshal(&document)
	if err != nil {
		return err
	}
	if err := os.WriteFile(host.Compose, body, 0600); err != nil {
		return err
	}
	return d.Copy(host.Compose, host.SSH, host.RemoteDir+"/compose.yml")
}

// Only source references absent before pp pulled them belong to pp. Existing
// operator-owned tags stay untouched. A private cache preserves if_missing.
func (d Deployer) prepareSourceImages(plan Plan, host HostBundle) error {
	body, err := os.ReadFile(host.Compose)
	if err != nil {
		return err
	}
	var compose ComposeFile
	if err := yaml.Unmarshal(body, &compose); err != nil {
		return err
	}
	owned := map[string]string{}
	for _, service := range host.PullServices {
		reference := compose.Services[service].Image
		cache := sourceCacheReference(plan.Config.Project, service, reference)
		script := "set -eu; if docker image inspect " + shellQuote(reference) + " >/dev/null 2>&1; then echo external; else\n"
		if plan.Config.Services[service].Pull == "if_missing" {
			script += "if docker image inspect " + shellQuote(cache) + " >/dev/null 2>&1; then docker tag " + shellQuote(cache) + " " + shellQuote(reference) + "; fi\n"
		}
		script += "echo managed; fi"
		out, err := d.Runner.Output(d.Root, "ssh", host.SSH, script)
		if err != nil {
			return err
		}
		if out == "managed" {
			owned[service] = reference
		}
	}
	return writeJSON(filepath.Join(host.Path, "sources.json"), owned)
}

func (d Deployer) finishSourceImage(plan Plan, host HostBundle, service, reference, id string, manifest *imageManifest) error {
	body, err := os.ReadFile(filepath.Join(host.Path, "sources.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var owned map[string]string
	if err := json.Unmarshal(body, &owned); err != nil {
		return err
	}
	if owned[service] != reference {
		return nil
	}
	cache := sourceCacheReference(plan.Config.Project, service, reference)
	manifest.Images = append(manifest.Images, ManagedImage{Reference: cache, ID: id})
	if err := writeJSON(filepath.Join(host.Path, "images.json"), manifest); err != nil {
		return err
	}
	return d.Remote(host.SSH, "docker tag "+shellQuote(id)+" "+shellQuote(cache))
}

func yamlMapValue(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func sourceCacheReference(project Project, service, reference string) string {
	return fmt.Sprintf("%scache/%s:%x", managedImagePrefix(project), service, sha256.Sum256([]byte(reference)))
}

// Capture newly fetched references even after a partial multi-service pull.
// A release alias keeps each resolved image available independently of tags.
func (d Deployer) recordPulledSources(plan Plan, host HostBundle) error {
	body, err := os.ReadFile(filepath.Join(host.Path, "sources.json"))
	if err != nil {
		return err
	}
	var owned map[string]string
	if err := json.Unmarshal(body, &owned); err != nil {
		return err
	}
	path := filepath.Join(host.Path, "images.json")
	manifest, err := readImageManifest(path)
	if err != nil {
		return err
	}
	for _, item := range sortedMap(owned) {
		out, err := d.Runner.Output(d.Root, "ssh", host.SSH, "docker image inspect --format '{{.Id}}' "+shellQuote(item.Value)+" 2>/dev/null || true")
		if err != nil {
			return err
		}
		if out == "" {
			continue
		}
		if !imageIDPattern.MatchString(out) {
			return fmt.Errorf("invalid pulled image ID")
		}
		alias := managedImagePrefix(plan.Config.Project) + item.Key + ":" + strings.ToLower(plan.ReleaseID)
		manifest.Images = append(manifest.Images, ManagedImage{Reference: item.Value, ID: out, Source: true}, ManagedImage{Reference: alias, ID: out})
		if err := writeJSON(path, manifest); err != nil {
			return err
		}
		if err := d.Remote(host.SSH, "docker tag "+shellQuote(out)+" "+shellQuote(alias)); err != nil {
			return err
		}
	}
	return nil
}

func imageRemovalScript(images []ManagedImage) string {
	script := "set -eu\n"
	for _, image := range images {
		// Never force removal, and never remove a tag that now points elsewhere.
		script += "id=$(docker image inspect --format '{{.Id}}' " + shellQuote(image.Reference) + " 2>/dev/null || true)\n"
		script += "if [ \"$id\" = " + shellQuote(image.ID) + " ]; then docker image rm " + shellQuote(image.Reference) + "; fi\n"
		script += "tags=$(docker image inspect --format '{{json .RepoTags}}' " + shellQuote(image.ID) + " 2>/dev/null || true)\n"
		script += "case \"$tags\" in '[]'|'null') docker image rm " + shellQuote(image.ID) + " ;; esac\n"
	}
	return script
}

func (d Deployer) cleanupLocalImages(project Project, record ReleaseRecord, protected map[string]bool) error {
	m, err := readImageManifest(filepath.Join(record.BundlePath, "images.json"))
	if err != nil {
		return err
	}
	if len(m.Images) > 0 || len(record.ImageTags) > 0 || len(record.Images) > 0 {
		if _, err := d.Runner.Output(d.Root, "docker", "info", "--format", "{{.ID}}"); err != nil {
			return err
		}
	}
	// Recover a build interrupted between Docker completion and manifest save.
	// Old images without our build labels still cannot be adopted for deletion.
	tags := append([]string(nil), record.ImageTags...)
	for _, image := range record.Images {
		tags = append(tags, image.Tags...)
	}
	for _, tag := range tags {
		known := false
		for _, image := range m.Images {
			if image.Reference == tag {
				known = true
			}
		}
		if known {
			continue
		}
		format := "{{.Id}} {{index .Config.Labels \"pp.project\"}}:{{index .Config.Labels \"pp.environment\"}}:{{index .Config.Labels \"pp.release\"}}"
		out, err := d.Runner.Output(d.Root, "sh", "-c", "docker image inspect --format "+shellQuote(format)+" "+shellQuote(tag)+" 2>/dev/null || true")
		if err != nil {
			return err
		}
		fields := strings.Fields(out)
		if len(fields) == 2 && imageIDPattern.MatchString(fields[0]) && fields[1] == project.Name+":"+project.Environment+":"+record.ReleaseID {
			m.Images = append(m.Images, ManagedImage{Reference: tag, ID: fields[0], Built: true})
		}
	}
	if len(m.Images) == 0 {
		return nil
	} // Old unowned images are not safe to prune.
	// Only images labelled by this pp build are eligible. An ordinary app may
	// share a tag, so check the current ID again immediately before removal.
	for _, image := range m.Images {
		if protected[image.Reference+"@"+image.ID] {
			continue
		}
		if !image.Built {
			if !strings.HasPrefix(image.Reference, managedImagePrefix(project)) {
				return fmt.Errorf("invalid local image ownership")
			}
			if err := d.Runner.Run(d.Root, "sh", "-c", imageRemovalScript([]ManagedImage{image})); err != nil {
				return err
			}
			continue
		}
		label := "{{index .Config.Labels \"pp.project\"}}:{{index .Config.Labels \"pp.environment\"}}:{{index .Config.Labels \"pp.release\"}}"
		script := "set -eu; owner=$(docker image inspect --format " + shellQuote(label) + " " + shellQuote(image.ID) + " 2>/dev/null || true); if [ \"$owner\" = " + shellQuote(project.Name+":"+project.Environment+":"+record.ReleaseID) + " ]; then\n" + imageRemovalScript([]ManagedImage{image}) + "fi\n"
		if err := d.Runner.Run(d.Root, "sh", "-c", script); err != nil {
			return err
		}
	}
	return nil
}

func recordHostBuilds(host HostBundle, images []ImageBundle) error {
	if len(images) == 0 {
		return nil
	}
	local, err := readImageManifest(filepath.Join(filepath.Dir(filepath.Dir(host.Path)), "images.json"))
	if err != nil {
		return err
	}
	path := filepath.Join(host.Path, "images.json")
	manifest, err := readImageManifest(path)
	if err != nil {
		return err
	}
	for _, image := range images {
		for _, tag := range image.Tags {
			for _, built := range local.Images {
				if built.Reference == tag {
					manifest.Images = append(manifest.Images, built)
				}
			}
		}
	}
	return writeJSON(path, manifest)
}

func (d Deployer) cleanupRemoteRelease(project Project, releaseID string, host HostRecord) error {
	if host.RemoteDir != remoteReleaseDir(project, releaseID) {
		return fmt.Errorf("invalid cleanup directory")
	}
	script := remoteLockScript()
	if d.lockedHosts[host.SSH] {
		script = "set -eu; "
	}
	// Each component is a known relative path, never an arbitrary recorded root.
	parts := strings.Split(host.RemoteDir, "/")
	for i := range parts {
		script += "[ ! -L " + shellQuote(strings.Join(parts[:i+1], "/")) + " ] || exit 1; "
	}
	script += "active=$(docker ps -a --filter " + shellQuote("label=pp.project="+project.Name) + " --filter " + shellQuote("label=pp.environment="+project.Environment) + " --filter " + shellQuote("label=pp.release="+releaseID) + " -q); [ -z \"$active\" ] || { echo 'release still has containers; retaining it' >&2; exit 1; };\n"
	for _, image := range host.Images {
		if (!image.Built && !image.Source && !strings.HasPrefix(image.Reference, managedImagePrefix(project))) || !imageIDPattern.MatchString(image.ID) {
			return fmt.Errorf("invalid remote image ownership")
		}
		if image.Source {
			owned := false
			for _, pin := range host.Images {
				if strings.HasPrefix(pin.Reference, managedImagePrefix(project)) && pin.ID == image.ID {
					owned = true
				}
			}
			if !owned {
				return fmt.Errorf("source image lacks a pp ownership record")
			}
		}
		if image.Built {
			label := "{{index .Config.Labels \"pp.project\"}}:{{index .Config.Labels \"pp.environment\"}}:{{index .Config.Labels \"pp.release\"}}"
			script += "owner=$(docker image inspect --format " + shellQuote(label) + " " + shellQuote(image.ID) + " 2>/dev/null || true); if [ \"$owner\" = " + shellQuote(project.Name+":"+project.Environment+":"+releaseID) + " ]; then\n" + imageRemovalScript([]ManagedImage{image}) + "fi\n"
		} else {
			script += imageRemovalScript([]ManagedImage{image})
		}
	}
	script += "rm -rf -- " + shellQuote(host.RemoteDir) + "\n"
	ctx, cancel := context.WithTimeout(d.Runner.context(), 45*time.Second)
	defer cancel()
	runner := d.Runner
	runner.Context = ctx
	return runner.SSHScript(d.Root, host.SSH, script)
}

func (d Deployer) removeUploadedImages(host HostBundle, images []ImageBundle) error {
	for _, image := range images {
		if err := d.Remote(host.SSH, "rm -f -- "+shellQuote(host.RemoteDir+"/images/"+filepath.Base(image.Tar))); err != nil {
			return err
		}
	}
	return nil
}
