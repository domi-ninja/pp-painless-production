package deploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const CurrentConfigVersion = 1
const CurrentStateVersion = 2

// Missing versions are v1. Only explicit migration steps may advance a format.
func checkVersion(kind string, version, current int) error {
	if version < 0 || version > current {
		return fmt.Errorf("unsupported %s version %d; this pp supports up to %d; upgrade pp before continuing", kind, version, current)
	}
	return nil
}

func upgradeState(state *State) error {
	if err := checkVersion("state", state.Version, CurrentStateVersion); err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	project := Project{Name: state.Project, Environment: state.Environment}
	if state.Version == 1 {
		if state.ResourceID == "" {
			state.ResourceID = project.DeploymentID()
		}
		state.Version = 2
	}
	if state.ResourceID == "" {
		return fmt.Errorf("state v2 is missing resource_id")
	}
	project.resourceID = state.ResourceID
	return project.validate()
}

func upgradeRelease(record *ReleaseRecord) error {
	state := State{Version: record.Version, Project: record.Project, Environment: record.Environment, ResourceID: record.ResourceID}
	if err := upgradeState(&state); err != nil {
		return fmt.Errorf("release %s: %w", record.ReleaseID, err)
	}
	record.Version, record.ResourceID = state.Version, state.ResourceID
	return nil
}

// Keep legacy bundles in place. Copy only metadata into an atomically published
// namespace, leaving the original state and artifacts available for recovery.
func migrateState(root string, selected Project) error {
	base := filepath.Join(root, ".deploy")
	if err := os.MkdirAll(base, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(base, ".state.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	path := statePath(root, selected)
	body, err := os.ReadFile(path)
	if err == nil {
		var state State
		if err := json.Unmarshal(body, &state); err != nil {
			return fmt.Errorf("parse state %s: %w", path, err)
		}
		if err := selected.checkOwner(state.Project, state.Environment); err != nil {
			return err
		}
		originalVersion := state.Version
		if err := upgradeState(&state); err != nil {
			return err
		}
		if originalVersion != state.Version {
			if err := backupVersion(path, body); err != nil {
				return err
			}
			return writeJSON(path, state)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body, err = os.ReadFile(filepath.Join(base, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state State
	if err := json.Unmarshal(body, &state); err != nil {
		return fmt.Errorf("parse legacy state: %w", err)
	}
	if err := checkVersion("legacy state", state.Version, 1); err != nil {
		return err
	}
	owner := Project{Name: state.Project, Environment: state.Environment, resourceID: state.Project}
	if err := owner.validate(); err != nil {
		return fmt.Errorf("cannot identify legacy deployment owner: %w", err)
	}
	if owner.Name != selected.Name || owner.Environment != selected.Environment {
		return nil // Never adopt another environment's state.
	}
	state.ResourceID = owner.ResourceID()
	if err := upgradeState(&state); err != nil {
		return err
	}
	parent := filepath.Dir(owner.localDir(root))
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".migrate-"+owner.Environment+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	paths, err := filepath.Glob(filepath.Join(base, "releases", "*", "metadata.json"))
	if err != nil {
		return err
	}
	found := map[string]bool{}
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record ReleaseRecord
		if err := json.Unmarshal(body, &record); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if record.Project != owner.Name || record.Environment != owner.Environment {
			continue
		}
		if err := checkVersion("legacy release", record.Version, 1); err != nil {
			return err
		}
		record.ResourceID = owner.ResourceID()
		if err := upgradeRelease(&record); err != nil {
			return err
		}
		id := filepath.Base(filepath.Dir(path))
		if err := relocateRecordPaths(root, owner, id, &record); err != nil {
			return err
		}
		if err := validateRecordPaths(root, owner, id, record); err != nil {
			return err
		}
		dest := filepath.Join(stage, "releases", id, "metadata.json")
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if err := writeJSON(dest, record); err != nil {
			return err
		}
		found[id] = true
	}
	for _, id := range []string{state.CurrentReleaseID, state.PreviousReleaseID} {
		if id != "" && !found[id] {
			return fmt.Errorf("cannot migrate: release %s is missing or belongs to another deployment; original state is unchanged", id)
		}
	}
	if err := writeJSON(filepath.Join(stage, "state.json"), state); err != nil {
		return err
	}
	if err := os.Rename(stage, owner.localDir(root)); err != nil {
		return fmt.Errorf("publish migrated state without overwriting existing files: %w", err)
	}
	return nil
}

func backupVersion(path string, body []byte) error {
	file, err := os.OpenFile(path+".v1.bak", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, os.ErrExist) {
		backup, err := os.ReadFile(path + ".v1.bak")
		if err != nil {
			return err
		}
		if !bytes.Equal(backup, body) {
			return fmt.Errorf("backup %s.v1.bak differs from the v1 file; inspect it before retrying; original file is unchanged", path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}

func atomicWrite(path string, body []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".pp-write-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(body); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
