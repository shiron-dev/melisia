package lock_test

import (
	"errors"
	"os"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

func withTempLockDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
}

func TestAcquireAndRelease(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "plan")
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

	if err := lock.Release("test-host", info.ID); err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}
}

func TestAcquireAlreadyLocked(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = lock.Release("test-host", info.ID) }()

	_, err = lock.Acquire("test-host", "plan")
	if err == nil {
		t.Fatal("expected error when acquiring already-locked host")
	}

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestReleaseNotOwned(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = lock.ForceUnlock("test-host") }()

	err = lock.Release("test-host", "wrong-id")
	if err == nil {
		t.Fatal("expected error when releasing with wrong ID")
	}

	if info.ID == "wrong-id" {
		t.Skip("generated ID happened to match, skipping")
	}
}

func TestForceUnlock(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = info

	if err := lock.ForceUnlock("test-host"); err != nil {
		t.Fatalf("unexpected error force-unlocking: %v", err)
	}

	if lock.IsLocked("test-host") {
		t.Error("expected host to be unlocked after force-unlock")
	}
}

func TestForceUnlockNoLock(t *testing.T) {
	withTempLockDir(t)

	err := lock.ForceUnlock("no-such-host")
	if err == nil {
		t.Fatal("expected error when force-unlocking non-existent lock")
	}
}

func TestRead(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = lock.Release("test-host", info.ID) }()

	read, err := lock.Read("test-host")
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
	withTempLockDir(t)

	_, err := lock.Read("no-such-host")
	if err == nil {
		t.Fatal("expected error when reading non-existent lock")
	}

	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist, got %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	withTempLockDir(t)

	if lock.IsLocked("test-host") {
		t.Error("expected host to be unlocked initially")
	}

	info, err := lock.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defer func() { _ = lock.Release("test-host", info.ID) }()

	if !lock.IsLocked("test-host") {
		t.Error("expected host to be locked after acquire")
	}
}

func TestReleaseAlreadyGone(t *testing.T) {
	withTempLockDir(t)

	info, err := lock.Acquire("test-host", "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = lock.ForceUnlock("test-host")

	err = lock.Release("test-host", info.ID)
	if err != nil {
		t.Errorf("expected no error when releasing already-gone lock, got %v", err)
	}
}
