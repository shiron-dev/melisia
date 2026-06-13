package lock_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
)

var (
	errNoSuchFile   = errors.New("no such file")
	errNotSupported = errors.New("not supported")
)

// fakeClient is an in-memory remote.RemoteClient that emulates the remote
// filesystem behaviour the locker relies on: atomic create via `set -C`, cat,
// rm -f and `[ -e ]`.
type fakeClient struct {
	files map[string]string
}

func newFakeClient() *fakeClient {
	return &fakeClient{files: make(map[string]string)}
}

var _ remote.RemoteClient = (*fakeClient)(nil)

func (c *fakeClient) RunCommand(_ string, command string) (string, error) {
	quoted := extractQuoted(command)

	switch {
	case strings.HasPrefix(command, "if mkdir -p"):
		// quoted segments: [dir, "%s" (from printf '%s'), payload, lockPath, lockPath]
		const minArgs = 4
		if len(quoted) < minArgs {
			return "", nil
		}

		payload, lockPath := quoted[2], quoted[3]
		if existing, exists := c.files[lockPath]; exists {
			return "CMT_LOCK_HELD\n" + existing, nil
		}

		c.files[lockPath] = payload

		return "CMT_LOCK_OK\n", nil
	case strings.HasPrefix(command, "if [ -e "):
		if len(quoted) > 0 {
			if _, ok := c.files[quoted[0]]; ok {
				return "Y\n", nil
			}
		}

		return "N\n", nil
	default:
		return "", nil
	}
}

func (c *fakeClient) ReadFile(remotePath string) ([]byte, error) {
	data, ok := c.files[remotePath]
	if !ok {
		return nil, errNoSuchFile
	}

	return []byte(data), nil
}

func (c *fakeClient) Remove(remotePath string) error {
	delete(c.files, remotePath)

	return nil
}

func (c *fakeClient) Close() error                     { return nil }
func (c *fakeClient) WriteFile(string, []byte) error   { return nil }
func (c *fakeClient) MkdirAll(string) error            { return nil }
func (c *fakeClient) Stat(string) (fs.FileInfo, error) { return nil, errNotSupported }
func (c *fakeClient) StatDirMetadata(string) (*remote.DirMetadata, error) {
	return nil, errNotSupported
}
func (c *fakeClient) ListFilesRecursive(string) ([]string, error) { return nil, nil }

// extractQuoted returns the contents of every single-quoted segment in s.
func extractQuoted(s string) []string {
	var out []string

	rest := s
	for {
		start := strings.IndexByte(rest, '\'')
		if start < 0 {
			break
		}

		rest = rest[start+1:]

		end := strings.IndexByte(rest, '\'')
		if end < 0 {
			break
		}

		out = append(out, rest[:end])
		rest = rest[end+1:]
	}

	return out
}

// fakeFactory hands out a shared fakeClient so multiple locker calls observe
// the same remote state.
type fakeFactory struct {
	client *fakeClient
}

func (f fakeFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return f.client, nil
}

func testTarget() lock.Target {
	return lock.Target{
		Host:      config.HostEntry{Name: "host1"},
		Project:   "grafana",
		RemoteDir: "/opt/compose/grafana",
		LockPath:  "/opt/compose/grafana/.cmt.lock",
	}
}

func newTestLocker() (*lock.RemoteLocker, *fakeClient) {
	client := newFakeClient()

	return lock.NewRemote(fakeFactory{client: client}), client
}

func TestAcquireAndRelease(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(target, "plan")
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

	err = locker.Release(target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}
}

func TestAcquireAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(target, "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = locker.Acquire(target, "plan")
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestReleaseNotOwned(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.Release(target, "wrong-id")
	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}
}

func TestForceUnlock(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(target, "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlock(target)
	if err != nil {
		t.Fatalf("unexpected error force-unlocking: %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if locked {
		t.Error("expected lock to be released after force-unlock")
	}
}

func TestForceUnlockNoLock(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()

	err := locker.ForceUnlock(testTarget())
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	read, err := locker.Read(target)
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

	locker, _ := newTestLocker()

	_, err := locker.Read(testTarget())
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	locked, _ := locker.IsLocked(target)
	if locked {
		t.Error("expected target to be unlocked initially")
	}

	_, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ = locker.IsLocked(target)
	if !locked {
		t.Error("expected target to be locked after acquire")
	}
}

func TestAcquireCorruptedLockFile(t *testing.T) {
	t.Parallel()

	locker, client := newTestLocker()
	target := testTarget()

	client.files[target.LockPath] = "invalid-json"

	_, err := locker.Acquire(target, "plan")
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked wrapping, got %v", err)
	}
}

func TestForceUnlockWithIDSuccess(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlockWithID(target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if locked {
		t.Error("expected lock to be released after ForceUnlockWithID")
	}
}

func TestForceUnlockWithIDMismatch(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlockWithID(target, "wrong-id")
	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if !locked {
		t.Error("expected lock to remain after ID mismatch")
	}
}

func TestReleaseAlreadyGone(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = locker.ForceUnlock(target)

	err = locker.Release(target, info.ID)
	if err != nil {
		t.Errorf("expected no error when releasing already-gone lock, got %v", err)
	}
}
