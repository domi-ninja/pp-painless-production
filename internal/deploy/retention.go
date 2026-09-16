package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Zero means the finite default, never unlimited retention.
type Retention struct {
	Releases     int   `yaml:"releases"`
	Failed       int   `yaml:"failed"`
	MinFreeMB    int64 `yaml:"min_free_mb"`
	BuildCacheMB int64 `yaml:"build_cache_mb"`
}

func (r Retention) defaults() Retention {
	if r.Releases == 0 {
		r.Releases = 3
	}
	if r.Failed == 0 {
		r.Failed = 1
	}
	if r.MinFreeMB == 0 {
		r.MinFreeMB = 1024
	}
	if r.BuildCacheMB == 0 {
		r.BuildCacheMB = 10240
	}
	return r
}

func (r Retention) validate() error {
	r = r.defaults()
	if r.Releases < 2 || r.Failed < 1 || r.MinFreeMB < 1 || r.BuildCacheMB < 1 || r.MinFreeMB > 1<<40 || r.BuildCacheMB > 1<<40 {
		return fmt.Errorf("retention requires releases >= 2, failed >= 1 and positive min_free_mb/build_cache_mb")
	}
	return nil
}

type cleanupJob struct {
	ReleaseID string       `json:"release_id"`
	Hosts     []HostRecord `json:"hosts"`
}

type cleanupQueue struct {
	Version int          `json:"version"`
	Jobs    []cleanupJob `json:"jobs"`
}

func (d Deployer) releaseRecords(project Project) ([]ReleaseRecord, error) {
	paths, err := filepath.Glob(filepath.Join(project.localDir(d.Root), "releases", "*", "metadata.json"))
	if err != nil {
		return nil, err
	}
	var records []ReleaseRecord
	for _, path := range paths {
		rel, err := filepath.Rel(d.Root, path)
		if err != nil {
			return nil, err
		}
		if err := noSymlinkPath(d.Root, rel); err != nil {
			return nil, err
		}
		record, err := LoadRelease(d.Root, project, filepath.Base(filepath.Dir(path)))
		if err != nil {
			return nil, err
		}
		for _, host := range record.Hosts {
			if !slugPattern.MatchString(host.ID) {
				return nil, fmt.Errorf("invalid recorded host ID")
			}
			if err := validateSSHTarget(host.SSH); err != nil {
				return nil, err
			}
		}
		records = append(records, record)
	}
	return records, nil
}

func expiredReleases(records []ReleaseRecord, state State, retention Retention) []ReleaseRecord {
	retention = retention.defaults()
	records = append([]ReleaseRecord(nil), records...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].ReleaseID > records[j].ReleaseID
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	// Count pinned releases first, even after rollback to an older release.
	successes, failures := 0, 0
	for _, r := range records {
		if r.ReleaseID == state.CurrentReleaseID || r.ReleaseID == state.PreviousReleaseID {
			successes++
		}
	}
	var expired []ReleaseRecord
	for _, r := range records {
		if r.ReleaseID == state.CurrentReleaseID || r.ReleaseID == state.PreviousReleaseID {
			continue
		}
		if r.Status == "ok" {
			successes++
			if successes <= retention.Releases {
				continue
			}
		} else {
			failures++
			if failures <= retention.Failed {
				continue
			}
		}
		expired = append(expired, r)
	}
	return expired
}

// Reject symlinks in every component, including links that stay inside the repo.
// Cleanup never follows a release path supplied by a symlink.
func noSymlinkPath(root, relative string) error {
	if err := validateRepoPath(root, relative); err != nil {
		return err
	}
	path := root
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing cleanup through symlink %s", path)
		}
	}
	return nil
}

func (d Deployer) readCleanupQueue(project Project) (cleanupQueue, error) {
	q := cleanupQueue{Version: 1}
	path := filepath.Join(project.localDir(d.Root), "cleanup.json")
	rel, err := filepath.Rel(d.Root, path)
	if err != nil {
		return q, err
	}
	if err := noSymlinkPath(d.Root, rel); err != nil {
		return q, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return q, nil
	}
	if err != nil {
		return q, err
	}
	if err := json.Unmarshal(data, &q); err != nil {
		return q, err
	}
	if q.Version != 1 {
		return q, fmt.Errorf("unsupported cleanup queue version %d", q.Version)
	}
	for _, job := range q.Jobs {
		if err := validateReleaseID(job.ReleaseID); err != nil {
			return q, err
		}
		for _, host := range job.Hosts {
			if host.RemoteDir != remoteReleaseDir(project, job.ReleaseID) {
				return q, fmt.Errorf("invalid pending cleanup directory")
			}
			if err := validateSSHTarget(host.SSH); err != nil {
				return q, err
			}
		}
	}
	return q, nil
}

// Called only while holding the operation lock. Remote work is journaled before
// local deletion, so an unavailable or removed host can be retried later.
func (d Deployer) cleanup(plan Plan, dryRun bool) error {
	project := plan.Config.Project
	state, err := LoadState(d.Root, project)
	if err != nil {
		return err
	}
	for _, id := range []string{state.CurrentReleaseID, state.PreviousReleaseID} {
		if id != "" {
			if _, err := LoadRelease(d.Root, project, id); err != nil {
				return err
			}
		}
	}
	records, err := d.releaseRecords(project)
	if err != nil {
		return err
	}
	q, err := d.readCleanupQueue(project)
	if err != nil {
		return err
	}
	expired := expiredReleases(records, state, plan.Config.Retention)
	expiredIDs := map[string]bool{}
	for _, record := range expired {
		expiredIDs[record.ReleaseID] = true
	}
	protected := map[string]map[string]bool{}
	localProtected := map[string]bool{}
	for _, record := range records {
		if expiredIDs[record.ReleaseID] {
			continue
		}
		local, err := readImageManifest(filepath.Join(record.BundlePath, "images.json"))
		if err != nil {
			return err
		}
		for _, image := range local.Images {
			localProtected[image.Reference+"@"+image.ID] = true
		}
		for _, host := range record.Hosts {
			manifest, err := readImageManifest(filepath.Join(record.BundlePath, "hosts", host.ID, "images.json"))
			if err != nil {
				return err
			}
			if protected[host.SSH] == nil {
				protected[host.SSH] = map[string]bool{}
			}
			for _, image := range manifest.Images {
				protected[host.SSH][image.Reference+"@"+image.ID] = true
			}
		}
	}
	queued := map[string]bool{}
	for _, job := range q.Jobs {
		queued[job.ReleaseID] = true
	}
	for _, record := range expired {
		for _, path := range []string{record.BundlePath, filepath.Dir(releaseMetadataPath(d.Root, project, record.ReleaseID))} {
			rel, err := filepath.Rel(d.Root, path)
			if err != nil {
				return err
			}
			if err := noSymlinkPath(d.Root, rel); err != nil {
				return err
			}
		}
		if !queued[record.ReleaseID] {
			for i := range record.Hosts {
				m, err := readImageManifest(filepath.Join(record.BundlePath, "hosts", record.Hosts[i].ID, "images.json"))
				if err != nil {
					return err
				}
				record.Hosts[i].Images = m.Images
			}
			q.Jobs = append(q.Jobs, cleanupJob{ReleaseID: record.ReleaseID, Hosts: record.Hosts})
			queued[record.ReleaseID] = true
		}
		fmt.Fprintf(d.Out, "cleanup: release %s (%s)\n", record.ReleaseID, record.Status)
		manifest, err := readImageManifest(filepath.Join(record.BundlePath, "images.json"))
		if err != nil {
			return err
		}
		for _, image := range manifest.Images {
			fmt.Fprintf(d.Out, "cleanup: local image %s (%s)\n", image.Reference, image.ID)
		}
		if len(manifest.Images) == 0 && len(record.ImageTags) > 0 {
			fmt.Fprintf(d.Out, "cleanup: legacy image tags have no ownership manifest; leaving Docker images unchanged\n")
		}
	}
	queuePath := filepath.Join(project.localDir(d.Root), "cleanup.json")
	if !dryRun {
		if err := writeJSON(queuePath, q); err != nil {
			return err
		}
	}
	var problems []error
	// Try each host independently. Do not let one offline server block others.
	for i := range q.Jobs {
		job := &q.Jobs[i]
		if job.ReleaseID == state.CurrentReleaseID || job.ReleaseID == state.PreviousReleaseID {
			return fmt.Errorf("pending cleanup targets a protected release %s", job.ReleaseID)
		}
		var pending []HostRecord
		for _, host := range job.Hosts {
			images := make([]ManagedImage, 0, len(host.Images))
			for _, image := range host.Images {
				if !protected[host.SSH][image.Reference+"@"+image.ID] {
					images = append(images, image)
				}
			}
			host.Images = images
			fmt.Fprintf(d.Out, "cleanup: %s:%s\n", host.SSH, host.RemoteDir)
			for _, image := range images {
				fmt.Fprintf(d.Out, "cleanup: %s image %s (%s)\n", host.SSH, image.Reference, image.ID)
			}
			if dryRun {
				pending = append(pending, host)
				continue
			}
			if err := d.cleanupRemoteRelease(project, job.ReleaseID, host); err != nil {
				problems = append(problems, err)
				pending = append(pending, host)
			}
		}
		job.Hosts = pending
	}
	if dryRun {
		return nil
	}
	for _, record := range expired {
		// An active remote container may still need this bundle for recovery.
		pending := false
		for _, job := range q.Jobs {
			if job.ReleaseID == record.ReleaseID && len(job.Hosts) > 0 {
				pending = true
			}
		}
		if pending {
			// Keep small recovery metadata and compose/env files, but shed large
			// archives only when the remote release is known not to be active.
			continue
		}
		if err := d.cleanupLocalImages(project, record, localProtected); err != nil {
			problems = append(problems, err)
			continue
		}
		if err := os.RemoveAll(record.BundlePath); err != nil {
			return err
		}
		metadataDir := filepath.Dir(releaseMetadataPath(d.Root, project, record.ReleaseID))
		if metadataDir != record.BundlePath {
			if err := os.RemoveAll(metadataDir); err != nil {
				return err
			}
		}
		fmt.Fprintf(d.Out, "cleanup: removed local release %s (not recoverable)\n", record.ReleaseID)
	}
	remaining := q.Jobs[:0]
	for _, job := range q.Jobs {
		if len(job.Hosts) > 0 {
			remaining = append(remaining, job)
		}
	}
	q.Jobs = remaining
	if err := writeJSON(queuePath, q); err != nil {
		return err
	}
	return errors.Join(problems...)
}

func (d Deployer) Cleanup(dryRun bool) error {
	unlock, err := d.operationLock()
	if err != nil {
		return err
	}
	defer unlock()
	plan, err := d.cleanupPlan()
	if err != nil {
		return err
	}
	cleanupErr := d.cleanup(plan, dryRun)
	cacheErr := d.cleanupBuildCache(plan.Config.Retention, dryRun)
	return errors.Join(cleanupErr, cacheErr)
}

// Recovery cleanup needs identity and retention, not build inputs, Git or secrets.
func (d Deployer) cleanupPlan() (Plan, error) {
	if err := validateRepoPath(d.Root, d.configPath()); err != nil {
		return Plan{}, err
	}
	body, err := os.ReadFile(filepath.Join(d.Root, d.configPath()))
	if err != nil {
		return Plan{}, err
	}
	var cfg struct {
		Version   int       `yaml:"version"`
		Project   Project   `yaml:"project"`
		Retention Retention `yaml:"retention"`
	}
	cfg.Version = CurrentConfigVersion
	if err := yaml.Unmarshal(body, &cfg); err != nil {
		return Plan{}, err
	}
	if cfg.Version != CurrentConfigVersion {
		return Plan{}, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	if err := cfg.Project.validate(); err != nil {
		return Plan{}, err
	}
	if err := cfg.Retention.validate(); err != nil {
		return Plan{}, err
	}
	state, err := LoadState(d.Root, cfg.Project)
	if err != nil {
		return Plan{}, err
	}
	cfg.Project.resourceID = state.ResourceID
	return Plan{Config: Config{Version: cfg.Version, Project: cfg.Project, Retention: cfg.Retention}}, nil
}
