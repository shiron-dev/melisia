package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
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

// writeTestRepoWithDirs builds on writeTestRepo, adding a host.yml that declares
// a project directory so plan builds DirPlans (whose remote existence is probed).
func writeTestRepoWithDirs(t *testing.T) string {
	t.Helper()

	configPath := writeTestRepo(t)
	base := filepath.Dir(configPath)

	hostDir := filepath.Join(base, "hosts", "test-host")

	err := os.MkdirAll(hostDir, 0o750)
	if err != nil {
		t.Fatalf("unexpected error creating host dir: %v", err)
	}

	hostContent := "projects:\n  grafana:\n    dirs:\n      - grafana_storage\n"

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(hostContent), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing host.yml: %v", err)
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

	err := runPlanCmd("/nonexistent/config.yml", nil, nil, false, "", syncer.PlanDependencies{})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

// plan is read-only and must not be blocked by a held lock from another
// operation.
func TestRunPlanCmdSucceedsWhenLocked(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	client := &fakeClient{files: make(map[string]string)}
	deps := planDeps(client)

	// Pre-lock the grafana project; plan must still succeed.
	preLocker := lock.NewRemote(fakeFactory{client: client})

	_, err := preLocker.Acquire(lock.Target{
		Host:      config.HostEntry{Name: "test-host"},
		Project:   "grafana",
		RemoteDir: "/opt/compose/grafana",
		LockPath:  "/opt/compose/grafana/.cmt.lock",
	}, "existing-op", true)
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	err = runPlanCmd(configPath, nil, nil, false, "", deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	}, "existing-op", true)
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

func TestRunPlanCmdSuccess(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)
	client := &fakeClient{files: make(map[string]string)}

	err := runPlanCmd(configPath, nil, nil, false, "", planDeps(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// withStubbedPlanExit replaces planExit with a recorder and restores it.
func withStubbedPlanExit(t *testing.T) *int {
	t.Helper()

	original := planExit

	var code int

	gotExit := false
	planExit = func(c int) {
		code = c
		gotExit = true
	}

	t.Cleanup(func() {
		planExit = original

		if !gotExit {
			t.Error("expected planExit to be called")
		}
	})

	return &code
}

func TestRunPlanCmdExitCodeNoChanges(t *testing.T) { //nolint:paralleltest // mutates the planExit global
	configPath := writeTestRepo(t)
	client := &fakeClient{files: make(map[string]string)}

	code := withStubbedPlanExit(t)

	err := runPlanCmd(configPath, nil, nil, true, "", planDeps(client))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *code != exitCodeNoChanges {
		t.Errorf("exit code = %d, want %d", *code, exitCodeNoChanges)
	}
}

func TestExitWithPlanCode(t *testing.T) { //nolint:paralleltest // mutates the planExit global
	tests := []struct {
		name string
		plan *syncer.SyncPlan
		want int
	}{
		{name: "no changes", plan: &syncer.SyncPlan{}, want: exitCodeNoChanges},
		{
			name: "has changes",
			plan: &syncer.SyncPlan{HostPlans: []syncer.HostPlan{{
				Projects: []syncer.ProjectPlan{{
					Files: []syncer.FilePlan{{Action: syncer.ActionAdd}},
				}},
			}}},
			want: exitCodeHasChanges,
		},
	}

	for _, testCase := range tests { //nolint:paralleltest // subtests mutate the planExit global
		t.Run(testCase.name, func(t *testing.T) {
			code := withStubbedPlanExit(t)

			exitWithPlanCode(testCase.plan)

			if *code != testCase.want {
				t.Errorf("exit code = %d, want %d", *code, testCase.want)
			}
		})
	}
}

func TestRunPlanCmdDigestWriteError(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)
	client := &fakeClient{files: make(map[string]string)}

	// A directory path can't be written as a file, so the digest write fails.
	digestPath := t.TempDir()

	err := runPlanCmd(configPath, nil, nil, false, digestPath, planDeps(client))
	if err == nil {
		t.Fatal("expected error when digest file path is not writable")
	}
}

// plan surfaces ErrExistenceCheckFailed when a directory's remote existence
// could not be determined (e.g. SSH unreachable).
func TestRunPlanCmdExistenceUnknown(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepoWithDirs(t)
	client := &fakeClient{files: make(map[string]string), statErr: remote.ErrExistenceUnknown}

	err := runPlanCmd(configPath, nil, nil, false, "", planDeps(client))
	if !errors.Is(err, syncer.ErrExistenceCheckFailed) {
		t.Fatalf("expected ErrExistenceCheckFailed, got %v", err)
	}
}

func TestRunApplyCmdSuccessNoChanges(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)
	client := &fakeClient{files: make(map[string]string)}

	err := runApplyCmd(configPath, nil, nil, true, false, "",
		syncer.ApplyDependencies{
			ClientFactory: fakeFactory{client: client},
			SSHResolver:   noopResolver{},
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSingleLockTargetSuccess(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	cfg, err := config.LoadCmtConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	target, err := resolveSingleLockTarget(cfg, "test-host", "grafana",
		syncer.PlanDependencies{SSHResolver: noopResolver{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if target.LockPath != "/opt/compose/grafana/.cmt.lock" {
		t.Errorf("lock path = %q, want /opt/compose/grafana/.cmt.lock", target.LockPath)
	}
}

func TestResolveSingleLockTargetNotFound(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	cfg, err := config.LoadCmtConfig(configPath)
	if err != nil {
		t.Fatalf("unexpected error loading config: %v", err)
	}

	_, err = resolveSingleLockTarget(cfg, "test-host", "nonexistent",
		syncer.PlanDependencies{SSHResolver: noopResolver{}})
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
}

func TestRunForceUnlockConfigNotFound(t *testing.T) {
	t.Parallel()

	err := runForceUnlock("/nonexistent/config.yml", "test-host", "grafana", false)
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestRunForceUnlockWithLockerForceWithInfo(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	// --force skips the prompt; info is read and ForceUnlockWithID is used.
	err = runForceUnlockWithLocker(locker, target, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if locked {
		t.Error("expected lock to be released")
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
