package deploy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReleaseRecord struct {
	Version           int           `json:"version"`
	ResourceID        string        `json:"resource_id"`
	Project           string        `json:"project"`
	Environment       string        `json:"environment"`
	ReleaseID         string        `json:"release_id"`
	PreviousReleaseID string        `json:"previous_release_id,omitempty"`
	Git               GitMetadata   `json:"git"`
	ImageTags         []string      `json:"image_tags"`
	ImageTar          string        `json:"image_tar"`
	Images            []ImageRecord `json:"images,omitempty"`
	BundlePath        string        `json:"bundle_path"`
	RemoteBase        string        `json:"remote_base"`
	Hosts             []HostRecord  `json:"hosts"`
	Migration         StepRecord    `json:"migration"`
	Apply             StepRecord    `json:"apply"`
	Rollback          StepRecord    `json:"rollback"`
	Checks            []CheckRecord `json:"checks,omitempty"`
	Status            string        `json:"status"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
}

type ImageRecord struct {
	ID   string   `json:"id"`
	Tar  string   `json:"tar"`
	Tags []string `json:"tags"`
}

type HostRecord struct {
	ID        string   `json:"id"`
	SSH       string   `json:"ssh"`
	Services  []string `json:"services"`
	RemoteDir string   `json:"remote_dir"`
	Status    string   `json:"status"`
}

type StepRecord struct {
	Status      string    `json:"status"`
	Error       string    `json:"error,omitempty"`
	At          time.Time `json:"at,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	Command     []string  `json:"command,omitempty"`
	Image       string    `json:"image,omitempty"`
	EnvSource   string    `json:"env_source,omitempty"`
	RestorePlan string    `json:"restore_plan,omitempty"`
}

type CheckRecord struct {
	Group           string    `json:"group"`
	Name            string    `json:"name"`
	Type            string    `json:"type"`
	Target          string    `json:"target"`
	ExpectedStatus  int       `json:"expected_status,omitempty"`
	ActualStatus    int       `json:"actual_status,omitempty"`
	FollowRedirects bool      `json:"follow_redirects,omitempty"`
	EnvSource       string    `json:"env_source,omitempty"`
	Status          string    `json:"status"`
	Error           string    `json:"error,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	DurationMS      int64     `json:"duration_ms"`
}

type State struct {
	Version           int       `json:"version"`
	ResourceID        string    `json:"resource_id"`
	Project           string    `json:"project"`
	Environment       string    `json:"environment"`
	CurrentReleaseID  string    `json:"current_release_id,omitempty"`
	PreviousReleaseID string    `json:"previous_release_id,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func LoadState(root string, project Project) (State, error) {
	if err := project.validate(); err != nil {
		return State{}, err
	}
	if err := migrateState(root, project); err != nil {
		return State{}, err
	}
	body, err := os.ReadFile(statePath(root, project))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return State{}, fmt.Errorf("parse state: %w", err)
	}
	if err := project.checkOwner(state.Project, state.Environment); err != nil {
		return State{}, err
	}
	if err := upgradeState(&state); err != nil {
		return State{}, err
	}
	return state, nil
}

func SaveState(root string, state State) error {
	if err := upgradeState(&state); err != nil {
		return err
	}
	project := Project{Name: state.Project, Environment: state.Environment}
	if err := project.validate(); err != nil {
		return err
	}
	state.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(project.localDir(root), 0755); err != nil {
		return fmt.Errorf("create deployment state directory: %w", err)
	}
	return writeJSON(statePath(root, project), state)
}

func LoadRelease(root string, project Project, releaseID string) (ReleaseRecord, error) {
	if err := project.validate(); err != nil {
		return ReleaseRecord{}, err
	}
	if err := validateReleaseID(releaseID); err != nil {
		return ReleaseRecord{}, err
	}
	state, err := LoadState(root, project)
	if err != nil {
		return ReleaseRecord{}, err
	}
	if project.resourceID == "" {
		project.resourceID = state.ResourceID
	}
	body, err := os.ReadFile(releaseMetadataPath(root, project, releaseID))
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("read release %s: %w", releaseID, err)
	}
	var record ReleaseRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return ReleaseRecord{}, fmt.Errorf("parse release %s: %w", releaseID, err)
	}
	if err := project.checkOwner(record.Project, record.Environment); err != nil {
		return ReleaseRecord{}, err
	}
	originalVersion := record.Version
	if err := upgradeRelease(&record); err != nil {
		return ReleaseRecord{}, err
	}
	if record.ResourceID != project.ResourceID() {
		return ReleaseRecord{}, fmt.Errorf("release %s has a mismatched resource identity", releaseID)
	}
	if err := relocateRecordPaths(root, project, releaseID, &record); err != nil {
		return ReleaseRecord{}, err
	}
	if err := validateRecordPaths(root, project, releaseID, record); err != nil {
		return ReleaseRecord{}, err
	}
	if originalVersion != record.Version {
		path := releaseMetadataPath(root, project, releaseID)
		if err := backupVersion(path, body); err != nil {
			return ReleaseRecord{}, err
		}
		if err := writeJSON(path, record); err != nil {
			return ReleaseRecord{}, err
		}
	}
	return record, nil
}

// Resolve a moved checkout using the deployment identity and release directory suffix.
// Paths are rebased together; artifacts outside the recorded bundle are never adopted.
// This leaves on-disk metadata untouched until an operation saves the release again.
func relocateRecordPaths(root string, project Project, releaseID string, record *ReleaseRecord) error {
	if record.ReleaseID != releaseID {
		return fmt.Errorf("release %s has a mismatched ID", releaseID)
	}
	oldBundle := filepath.Clean(record.BundlePath)
	suffixes := []string{filepath.Join(".deploy", project.Name, project.Environment, "releases", releaseID)}
	if project.ResourceID() == project.Name {
		suffixes = append(suffixes, filepath.Join(".deploy", "releases", releaseID))
	}
	var bundle string
	for _, suffix := range suffixes {
		if oldBundle == filepath.Join(root, suffix) || (filepath.IsAbs(oldBundle) && strings.HasSuffix(oldBundle, string(filepath.Separator)+suffix)) {
			bundle = filepath.Join(root, suffix)
			break
		}
	}
	if bundle == "" {
		return fmt.Errorf("release %s has a mismatched bundle path", releaseID)
	}
	rebase := func(path string) (string, error) {
		if path == "" {
			return "", nil
		}
		relative, err := filepath.Rel(oldBundle, path)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("release %s has an artifact outside its bundle: %s", releaseID, path)
		}
		return filepath.Join(bundle, relative), nil
	}
	var err error
	if record.ImageTar, err = rebase(record.ImageTar); err != nil {
		return err
	}
	for i := range record.Images {
		if record.Images[i].Tar, err = rebase(record.Images[i].Tar); err != nil {
			return err
		}
	}
	record.BundlePath = bundle
	return nil
}

func validateRecordPaths(root string, project Project, releaseID string, record ReleaseRecord) error {
	bundlePath := filepath.Clean(record.BundlePath)
	validBundle := bundlePath == filepath.Join(project.localDir(root), "releases", releaseID)
	if project.ResourceID() == project.Name {
		validBundle = validBundle || bundlePath == filepath.Join(root, ".deploy", "releases", releaseID)
	}
	if record.ReleaseID != releaseID || !validBundle {
		return fmt.Errorf("release %s has a mismatched ID or bundle path", releaseID)
	}
	for _, host := range record.Hosts {
		if host.RemoteDir != remoteReleaseDir(project, releaseID) {
			return fmt.Errorf("release %s has a mismatched remote directory", releaseID)
		}
	}
	return nil
}

func SaveRelease(root string, record ReleaseRecord) error {
	if err := upgradeRelease(&record); err != nil {
		return err
	}
	project := Project{Name: record.Project, Environment: record.Environment}
	if err := project.validate(); err != nil {
		return err
	}
	if err := validateReleaseID(record.ReleaseID); err != nil {
		return err
	}
	record.UpdatedAt = time.Now().UTC()
	path := releaseMetadataPath(root, project, record.ReleaseID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create release metadata dir: %w", err)
	}
	return writeJSON(path, record)
}

func statePath(root string, project Project) string {
	return filepath.Join(project.localDir(root), "state.json")
}

func releaseMetadataPath(root string, project Project, releaseID string) string {
	return filepath.Join(project.localDir(root), "releases", releaseID, "metadata.json")
}

func validateReleaseID(id string) error {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return fmt.Errorf("invalid release ID %q", id)
	}
	return nil
}

func writeJSON(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	body = append(body, '\n')
	if err := atomicWrite(path, body); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
