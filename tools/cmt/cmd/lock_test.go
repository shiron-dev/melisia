package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"
)

func TestAcquireHostLocksSuccess(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	hosts := []config.HostEntry{{Name: "host1"}, {Name: "host2"}}

	var buf strings.Builder

	release, err := acquireHostLocks(locker, hosts, "plan", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !locker.IsLocked("host1") {
		t.Error("expected host1 to be locked")
	}

	if !locker.IsLocked("host2") {
		t.Error("expected host2 to be locked")
	}

	release()

	if locker.IsLocked("host1") {
		t.Error("expected host1 to be unlocked after release")
	}

	if locker.IsLocked("host2") {
		t.Error("expected host2 to be unlocked after release")
	}
}

func TestAcquireHostLocksAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	existing, err := locker.Acquire("host1", "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	defer func() { _ = locker.Release("host1", existing.ID) }()

	hosts := []config.HostEntry{{Name: "host1"}}

	var buf strings.Builder

	release, err := acquireHostLocks(locker, hosts, "plan", &buf)
	if err == nil {
		release()
		t.Fatal("expected error when host is already locked")
	}

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestAcquireHostLocksReleasesOnPartialFailure(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	existing, err := locker.Acquire("host2", "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring host2: %v", err)
	}

	defer func() { _ = locker.Release("host2", existing.ID) }()

	hosts := []config.HostEntry{{Name: "host1"}, {Name: "host2"}}

	var buf strings.Builder

	_, err = acquireHostLocks(locker, hosts, "plan", &buf)
	if err == nil {
		t.Fatal("expected error when second host is already locked")
	}

	if locker.IsLocked("host1") {
		t.Error("expected host1 to be released after partial failure")
	}
}

func TestAcquireHostLocksEmpty(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	var buf strings.Builder

	release, err := acquireHostLocks(locker, nil, "plan", &buf)
	if err != nil {
		t.Fatalf("unexpected error for empty host list: %v", err)
	}

	release()
}

func TestRunForceUnlockWithLockerNotFound(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	err := runForceUnlockWithLocker(locker, "no-such-host", false)
	if err == nil {
		t.Fatal("expected error for non-existent lock")
	}

	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRunForceUnlockWithLockerCorruptedNoForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	locker := lock.NewWithDir(dir)

	lockPath := filepath.Join(dir, "bad-host.lock")

	err := os.WriteFile(lockPath, []byte("not-json"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing corrupted lock file: %v", err)
	}

	err = runForceUnlockWithLocker(locker, "bad-host", false)
	if err == nil {
		t.Fatal("expected error for corrupted lock without --force")
	}
}

func TestRunForceUnlockWithLockerCorruptedWithForce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	locker := lock.NewWithDir(dir)

	lockPath := filepath.Join(dir, "bad-host.lock")

	err := os.WriteFile(lockPath, []byte("not-json"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing corrupted lock file: %v", err)
	}

	err = runForceUnlockWithLocker(locker, "bad-host", true)
	if err != nil {
		t.Fatalf("expected no error for corrupted lock with --force, got %v", err)
	}

	if locker.IsLocked("bad-host") {
		t.Error("expected lock to be removed after force-unlock")
	}
}

func TestRunForceUnlockWithLockerSuccess(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	_, err := locker.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	err = runForceUnlockWithLocker(locker, "test-host", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if locker.IsLocked("test-host") {
		t.Error("expected host to be unlocked after force-unlock")
	}
}

func TestRunForceUnlockWithLockerIDChangedAfterConfirm(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	_ = locker.ForceUnlock("test-host")

	_, err = locker.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error re-acquiring lock: %v", err)
	}

	defer func() { _ = locker.ForceUnlock("test-host") }()

	err = locker.ForceUnlockWithID("test-host", info.ID)
	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}

	if !locker.IsLocked("test-host") {
		t.Error("expected new lock to remain after ID mismatch")
	}
}

func TestRunForceUnlockWithLockerCancelConfirm(t *testing.T) { //nolint:paralleltest
	locker := lock.NewWithDir(t.TempDir())

	_, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	defer func() { _ = locker.ForceUnlock("test-host") }()

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("unexpected error creating pipe: %v", pipeErr)
	}

	_, _ = w.WriteString("n\n")
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })

	err = runForceUnlockWithLocker(locker, "test-host", false)
	if err != nil {
		t.Fatalf("unexpected error for cancelled force-unlock: %v", err)
	}

	if !locker.IsLocked("test-host") {
		t.Error("expected host to still be locked after cancelled force-unlock")
	}
}

func TestConfirmForceUnlockYes(t *testing.T) { //nolint:paralleltest
	for _, answer := range []string{"y\n", "yes\n", "YES\n", "Y\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("unexpected error creating pipe: %v", err)
		}

		_, _ = w.WriteString(answer)
		_ = w.Close()

		oldStdin := os.Stdin
		os.Stdin = r

		t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })

		if !confirmForceUnlock("host") {
			t.Errorf("expected confirmForceUnlock to return true for %q", answer)
		}
	}
}

func TestConfirmForceUnlockNo(t *testing.T) { //nolint:paralleltest
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("unexpected error creating pipe: %v", err)
	}

	_, _ = w.WriteString("n\n")
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })

	if confirmForceUnlock("host") {
		t.Error("expected confirmForceUnlock to return false for 'n'")
	}
}

func TestRunPlanCmdConfigNotFound(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	err := runPlanCmdWithLocker(locker, "/nonexistent/config.yml", nil, nil, false, "", syncer.PlanDependencies{})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestRunPlanCmdWithLockerLockFail(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	locker := lock.NewWithDir(dir)

	info, err := locker.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	defer func() { _ = locker.Release("test-host", info.ID) }()

	configPath := filepath.Join(dir, "cmt.yml")
	configContent := "basePath: ./\nhosts:\n  - name: test-host\n    host: localhost\n    user: root\n    port: 22\n"

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing config file: %v", err)
	}

	err = runPlanCmdWithLocker(locker, configPath, nil, nil, false, "", syncer.PlanDependencies{})
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestRunForceUnlockWrapperNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	err := runForceUnlock("no-such-host", false)
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRunPlanCmdWrapperConfigNotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	err := runPlanCmd("/nonexistent/config.yml", nil, nil, false, "", syncer.PlanDependencies{})
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestNewForceUnlockCmdExecution(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	cmd := newForceUnlockCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{"no-such-host", "--force"})

	err := cmd.Execute()
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestNewApplyCmdLockFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	lockDir := filepath.Join(dir, "cmt", "locks")

	err := os.MkdirAll(lockDir, 0o700)
	if err != nil {
		t.Fatalf("unexpected error creating lock dir: %v", err)
	}

	locker := lock.NewWithDir(lockDir)

	info, err := locker.Acquire("test-host", "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	defer func() { _ = locker.Release("test-host", info.ID) }()

	configPath := filepath.Join(dir, "cmt.yml")
	configContent := "basePath: ./\nhosts:\n  - name: test-host\n    host: localhost\n    user: root\n    port: 22\n"

	err = os.WriteFile(configPath, []byte(configContent), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing config file: %v", err)
	}

	cmd := newApplyCmd(&configPath)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err = cmd.Execute()
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
