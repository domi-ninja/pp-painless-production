package deploy

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Migration images also live in the local Docker store. Keep a private alias
// and record the exact ID so rollback does not pick the first application build.
func (d Deployer) pinMigrationImage(plan Plan, reference string) (string, error) {
	path := filepath.Join(plan.Config.Project.localDir(d.Root), "releases", plan.ReleaseID, "images.json")
	manifest, err := readImageManifest(path)
	if err != nil {
		return "", err
	}
	alias := managedImagePrefix(plan.Config.Project) + "migrations/runner:" + strings.ToLower(plan.ReleaseID)
	for _, image := range manifest.Images {
		if image.Reference == alias {
			return image.ID, nil
		}
	}
	if _, err := d.Runner.Output(d.Root, "docker", "info", "--format", "{{.ID}}"); err != nil {
		return "", err
	}
	inspect := func(tag string) (string, error) {
		return d.Runner.Output(d.Root, "sh", "-c", "docker image inspect --format '{{.Id}}' "+shellQuote(tag)+" 2>/dev/null || true")
	}
	id, err := inspect(reference)
	if err != nil {
		return "", err
	}
	cache := sourceCacheReference(plan.Config.Project, "migration", reference)
	managed := id == ""
	pulled := false
	if managed {
		id, err = inspect(cache)
		if err != nil {
			return "", err
		}
		if id == "" {
			if err := d.Runner.Run(d.Root, "docker", "pull", reference); err != nil {
				return "", err
			}
			pulled = true
			id, err = inspect(reference)
			if err != nil {
				return "", err
			}
		}
	}
	if !imageIDPattern.MatchString(id) {
		return "", fmt.Errorf("invalid migration image ID")
	}
	manifest.Images = append(manifest.Images, ManagedImage{Reference: alias, ID: id})
	if managed {
		manifest.Images = append(manifest.Images, ManagedImage{Reference: cache, ID: id})
	}
	if err := writeJSON(path, manifest); err != nil {
		return "", err
	}
	if err := d.Runner.Run(d.Root, "docker", "tag", id, alias); err != nil {
		return "", err
	}
	if managed {
		if err := d.Runner.Run(d.Root, "docker", "tag", id, cache); err != nil {
			return "", err
		}
	}
	if pulled {
		if err := d.Runner.Run(d.Root, "sh", "-c", imageRemovalScript([]ManagedImage{{Reference: reference, ID: id}})); err != nil {
			return "", err
		}
	}
	return id, nil
}
