package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"
)

// noopResolver satisfies config.SSHConfigResolver without invoking ssh.
type noopResolver struct{}

func (noopResolver) Resolve(entry *config.HostEntry, _, _ string) error {
	if entry.Port == 0 {
		entry.Port = 22
	}

	return nil
}

// writeTestRepo creates a minimal cmt repo (config.yml + one project dir) and
// returns the config path.
func writeTestRepo(t *testing.T) string {
	t.Helper()

	base := t.TempDir()

	err := os.MkdirAll(filepath.Join(base, "projects", "grafana"), 0o750)
	if err != nil {
		t.Fatalf("unexpected error creating project dir: %v", err)
	}

	configPath := filepath.Join(base, "config.yml")
	configContent := "basePath: .\n" +
		"defaults:\n  remotePath: /opt/compose\n" +
		"hosts:\n  - name: test-host\n    host: localhost\n    user: root\n    port: 22\n"

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing config: %v", err)
	}

	return configPath
}

func planDeps(client *fakeClient) syncer.PlanDependencies {
	return syncer.PlanDependencies{
		ClientFactory: fakeFactory{client: client},
		SSHResolver:   noopResolver{},
	}
}

func TestRunPlanCmdConfigNotFound(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	err := runPlanCmdWithLocker(locker, "/nonexistent/config.yml", nil, nil, false, "", syncer.PlanDependencies{})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestRunPlanCmdWithLockerLockFail(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	client := &fakeClient{files: make(map[string]string)}
	deps := planDeps(client)

	// Pre-lock the grafana project so plan's acquire fails.
	preLocker := lock.NewRemote(fakeFactory{client: client})

	_, err := preLocker.Acquire(lock.Target{
		Host:      config.HostEntry{Name: "test-host"},
		Project:   "grafana",
		RemoteDir: "/opt/compose/grafana",
		LockPath:  "/opt/compose/grafana/.cmt.lock",
	}, "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	locker := lock.NewRemote(fakeFactory{client: client})

	err = runPlanCmdWithLocker(locker, configPath, nil, nil, false, "", deps)
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestRunPlanCmdWrapperConfigNotFound(t *testing.T) {
	t.Parallel()

	err := runPlanCmd("/nonexistent/config.yml", nil, nil, false, "",
		syncer.PlanDependencies{ClientFactory: fakeFactory{client: &fakeClient{files: map[string]string{}}}})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestRunApplyCmdConfigNotFound(t *testing.T) {
	t.Parallel()

	err := runApplyCmd("/nonexistent/config.yml", nil, nil, true, false, "",
		syncer.ApplyDependencies{
			ClientFactory: fakeFactory{client: &fakeClient{files: map[string]string{}}},
			SSHResolver:   noopResolver{},
		})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestRunApplyCmdLockFail(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	client := &fakeClient{files: make(map[string]string)}

	preLocker := lock.NewRemote(fakeFactory{client: client})

	_, err := preLocker.Acquire(lock.Target{
		Host:      config.HostEntry{Name: "test-host"},
		Project:   "grafana",
		RemoteDir: "/opt/compose/grafana",
		LockPath:  "/opt/compose/grafana/.cmt.lock",
	}, "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	err = runApplyCmd(configPath, nil, nil, true, false, "",
		syncer.ApplyDependencies{
			ClientFactory: fakeFactory{client: client},
			SSHResolver:   noopResolver{},
		})
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestWritePlanDigestFileEmpty(t *testing.T) {
	t.Parallel()

	err := writePlanDigestFile("", &syncer.SyncPlan{})
	if err != nil {
		t.Fatalf("unexpected error for empty digestFile path: %v", err)
	}
}

func TestWritePlanDigestFile(t *testing.T) {
	t.Parallel()

	digestPath := filepath.Join(t.TempDir(), "digest.txt")

	err := writePlanDigestFile(digestPath, &syncer.SyncPlan{})
	if err != nil {
		t.Fatalf("unexpected error writing digest file: %v", err)
	}

	data, err := os.ReadFile(digestPath) //nolint:gosec
	if err != nil {
		t.Fatalf("unexpected error reading digest file: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty digest file")
	}

	if data[len(data)-1] != '\n' {
		t.Error("expected digest file to end with newline")
	}
}
