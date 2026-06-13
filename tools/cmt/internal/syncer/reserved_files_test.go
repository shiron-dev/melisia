package syncer

import (
	"slices"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"

	"go.uber.org/mock/gomock"
)

func TestBuildManifestExcludesReservedFiles(t *testing.T) {
	t.Parallel()

	localFiles := map[string]string{
		"compose.yml":       "/local/compose.yml",
		lock.LockFileName:   "/local/.cmt.lock",
		manifestFile:        "/local/.cmt-manifest.json",
		"files/config.yaml": "/local/files/config.yaml",
	}

	manifest := BuildManifest(localFiles)

	if slices.Contains(manifest.ManagedFiles, lock.LockFileName) {
		t.Errorf("manifest must not record the lock file: %v", manifest.ManagedFiles)
	}

	if slices.Contains(manifest.ManagedFiles, manifestFile) {
		t.Errorf("manifest must not record itself: %v", manifest.ManagedFiles)
	}

	if !slices.Contains(manifest.ManagedFiles, "compose.yml") {
		t.Errorf("expected compose.yml to be managed: %v", manifest.ManagedFiles)
	}
}

func TestBuildDeleteFilePlansSkipsLockFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	// A stale manifest that lists the lock file must not produce a delete plan,
	// so client.ReadFile is never called for it.
	manifest := &Manifest{ManagedFiles: []string{lock.LockFileName}}

	plans := buildDeleteFilePlans(manifest, map[string]bool{}, "/opt/compose/grafana", client, map[string]bool{})
	if len(plans) != 0 {
		t.Errorf("expected no delete plans for reserved lock file, got %d: %+v", len(plans), plans)
	}
}

func TestBuildFilePlansSkipsLocalLockFile(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	// A local file colliding with the lock name must be ignored entirely:
	// no remote read/write is attempted for it.
	localFiles := map[string]string{lock.LockFileName: "/local/.cmt.lock"}

	plans, err := buildFilePlans(localFiles, "/opt/compose/grafana", nil, client, nil, map[string]bool{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plans) != 0 {
		t.Errorf("expected no file plans for reserved lock file, got %d: %+v", len(plans), plans)
	}
}
