package lock_test

import (
	"errors"
	"os"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

func TestAcquireAndRelease(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	if info.ID == "" {
		t.Error("expected non-empty lock ID")
	}

	if info.Operation != "plan" {
		t.Errorf("expected operation %q, got %q", "plan", info.Operation)
	}

	if info.Who == "" {
		t.Error("expected non-empty who")
	}

	err = locker.Release("test-host", info.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}
}

func TestAcquireAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = locker.Release("test-host", info.ID) }()

	_, err = locker.Acquire("test-host", "plan")
	if err == nil {
		t.Fatal("expected error when acquiring already-locked host")
	}

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestReleaseNotOwned(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = locker.ForceUnlock("test-host") }()

	if info.ID == "wrong-id" {
		t.Skip("generated ID happened to match, skipping")
	}

	err = locker.Release("test-host", "wrong-id")
	if err == nil {
		t.Fatal("expected error when releasing with wrong ID")
	}
}

func TestForceUnlock(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	_, err := locker.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlock("test-host")
	if err != nil {
		t.Fatalf("unexpected error force-unlocking: %v", err)
	}

	if locker.IsLocked("test-host") {
		t.Error("expected host to be unlocked after force-unlock")
	}
}

func TestForceUnlockNoLock(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	err := locker.ForceUnlock("no-such-host")
	if err == nil {
		t.Fatal("expected error when force-unlocking non-existent lock")
	}

	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = locker.Release("test-host", info.ID) }()

	read, err := locker.Read("test-host")
	if err != nil {
		t.Fatalf("unexpected error reading lock: %v", err)
	}

	if read.ID != info.ID {
		t.Errorf("expected ID %q, got %q", info.ID, read.ID)
	}

	if read.Operation != info.Operation {
		t.Errorf("expected operation %q, got %q", info.Operation, read.Operation)
	}
}

func TestReadNoLock(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	_, err := locker.Read("no-such-host")
	if err == nil {
		t.Fatal("expected error when reading non-existent lock")
	}

	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	if locker.IsLocked("test-host") {
		t.Error("expected host to be unlocked initially")
	}

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = locker.Release("test-host", info.ID) }()

	if !locker.IsLocked("test-host") {
		t.Error("expected host to be locked after acquire")
	}
}

func TestAcquireCorruptedLockFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	locker := lock.NewWithDir(dir)

	err := os.WriteFile(dir+"/test-host.lock", []byte("invalid-json"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing corrupted lock: %v", err)
	}

	_, err = locker.Acquire("test-host", "plan")
	if err == nil {
		t.Fatal("expected error when lock file already exists")
	}

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked wrapping, got %v", err)
	}
}

func TestNewAndDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	locker := lock.NewWithDir(dir)

	if locker.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", locker.Dir(), dir)
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	locker := lock.New()
	if locker == nil {
		t.Error("New() returned nil")
	}

	if locker.Dir() == "" {
		t.Error("expected non-empty Dir() from New()")
	}
}

func TestForceUnlockWithIDSuccess(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlockWithID("test-host", info.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if locker.IsLocked("test-host") {
		t.Error("expected host to be unlocked after ForceUnlockWithID")
	}
}

func TestForceUnlockWithIDMismatch(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	_, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = locker.ForceUnlock("test-host") }()

	err = locker.ForceUnlockWithID("test-host", "wrong-id")
	if err == nil {
		t.Fatal("expected error for mismatched lock ID")
	}

	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}

	if !locker.IsLocked("test-host") {
		t.Error("expected host to remain locked after ID mismatch")
	}
}

func TestReleaseAlreadyGone(t *testing.T) {
	t.Parallel()

	locker := lock.NewWithDir(t.TempDir())

	info, err := locker.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = locker.ForceUnlock("test-host")

	err = locker.Release("test-host", info.ID)
	if err != nil {
		t.Errorf("expected no error when releasing already-gone lock, got %v", err)
	}
}
