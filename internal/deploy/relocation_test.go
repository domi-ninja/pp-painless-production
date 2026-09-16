package deploy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMovedCheckoutPreservesReleaseArtifacts(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "namespaced"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "original")
			project := Project{Name: "shop", Environment: "prod"}
			if legacy {
				project = legacyFixture(t, root)
			} else {
				for _, id := range []string{"current", "previous"} {
					bundle := filepath.Dir(releaseMetadataPath(root, project, id))
					record := ReleaseRecord{Project: project.Name, Environment: project.Environment, ReleaseID: id,
						BundlePath: bundle, ImageTar: filepath.Join(bundle, "images", "web.tar"),
						Images: []ImageRecord{{ID: "web", Tar: filepath.Join(bundle, "images", "web.tar")}},
						Hosts:  []HostRecord{{ID: "host", RemoteDir: remoteReleaseDir(project, id)}}}
					if err := SaveRelease(root, record); err != nil {
						t.Fatal(err)
					}
					writeNestedFile(t, bundle, "hosts/host/compose.yml", "services: {}\n")
				}
				if err := SaveState(root, State{Project: project.Name, Environment: project.Environment, CurrentReleaseID: "current", PreviousReleaseID: "previous"}); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []string{"current", "previous"} {
				record, err := LoadRelease(root, project, id)
				if err != nil {
					t.Fatal(err)
				}
				writeNestedFile(t, record.BundlePath, "images/web.tar", "image fixture")
			}
			for _, folder := range []string{"renamed", "moved-again"} {
				moved := filepath.Join(parent, folder)
				if err := os.Rename(root, moved); err != nil {
					t.Fatal(err)
				}
				root = moved
				for _, id := range []string{"current", "previous"} {
					path := releaseMetadataPath(root, project, id)
					before := mustRead(t, path)
					record, err := LoadRelease(root, project, id)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(before, mustRead(t, path)) {
						t.Fatal("load rewrote metadata")
					}
					bundle := bundleFromRecord(record)
					for _, artifact := range []string{bundle.ImageTar, bundle.Images[0].Tar, bundle.Hosts[0].Compose} {
						if _, err := os.Stat(artifact); err != nil {
							t.Fatal(err)
						}
					}
					// Saving and moving again must also work, including adopted legacy resources.
					if err := SaveRelease(root, record); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}

func TestMovedLegacyCheckoutBeforeMigration(t *testing.T) {
	parent := t.TempDir()
	old := filepath.Join(parent, "old")
	project := legacyFixture(t, old)
	moved := filepath.Join(parent, "new")
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	record, err := LoadRelease(moved, project, "previous")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(bundleFromRecord(record).Hosts[0].Compose); err != nil {
		t.Fatal(err)
	}
}

func TestRelocationRejectsArtifactsOutsideBundle(t *testing.T) {
	project := Project{Name: "shop", Environment: "dev"}
	old := "/old/.deploy/shop/dev/releases/release"
	for _, path := range []string{"/etc/passwd", old + "/../other/image.tar"} {
		record := ReleaseRecord{ReleaseID: "release", BundlePath: old, Images: []ImageRecord{{Tar: path}}}
		if err := relocateRecordPaths(t.TempDir(), project, "release", &record); err == nil {
			t.Fatal("accepted external artifact")
		}
	}
}
