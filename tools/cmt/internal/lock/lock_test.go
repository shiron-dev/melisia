package lock_test

import (
	"context"
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
	dirs  map[string]bool
}

func newFakeClient() *fakeClient {
	return &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
}

var _ remote.RemoteClient = (*fakeClient)(nil)

func (c *fakeClient) RunCommand(_ context.Context, _ string, command string) (string, error) {
	if c.dirs == nil {
		c.dirs = make(map[string]bool)
	}

	quoted := extractQuoted(command)

	switch {
	case strings.HasPrefix(command, "rmdir "): // roll back empty dir
		if len(quoted) > 0 && c.dirEmpty(quoted[0]) {
			delete(c.dirs, quoted[0])
		}

		return "", nil
	case strings.HasPrefix(command, "if [ -e "): // existence check
		if len(quoted) > 0 {
			if _, ok := c.files[quoted[0]]; ok {
				return "Y\n", nil
			}
		}

		return "N\n", nil
	case strings.Contains(command, "mkdir -p"): // apply: [dir, dir, "%s", payload, lock, lock]
		const minArgs = 5
		if len(quoted) < minArgs {
			return "", nil
		}

		created := "0"

		if !c.dirs[quoted[0]] {
			c.dirs[quoted[0]] = true
			created = "1"
		}

		return c.tryCreate(quoted[4], quoted[3], created), nil
	case strings.Contains(command, "if [ -d "): // plan: [dir, "%s", payload, lock, lock]
		const minArgs = 4

		if len(quoted) < minArgs {
			return "", nil
		}

		if !c.dirs[quoted[0]] {
			return "CMT_LOCK_NODIR\n", nil
		}

		return c.tryCreate(quoted[3], quoted[2], "0"), nil
	default:
		return "", nil
	}
}

func (c *fakeClient) ReadFile(_ context.Context, remotePath string) ([]byte, error) {
	data, ok := c.files[remotePath]
	if !ok {
		return nil, errNoSuchFile
	}

	return []byte(data), nil
}

func (c *fakeClient) Remove(_ context.Context, remotePath string) error {
	delete(c.files, remotePath)

	return nil
}

func (c *fakeClient) Close() error                                      { return nil }
func (c *fakeClient) WriteFile(context.Context, string, []byte) error   { return nil }
func (c *fakeClient) MkdirAll(context.Context, string) error            { return nil }
func (c *fakeClient) Stat(context.Context, string) (fs.FileInfo, error) { return nil, errNotSupported }
func (c *fakeClient) StatDirMetadata(context.Context, string) (*remote.DirMetadata, error) {
	return nil, errNotSupported
}
func (c *fakeClient) ListFilesRecursive(context.Context, string) ([]string, error) { return nil, nil }

func (c *fakeClient) tryCreate(lockPath, payload, created string) string {
	if existing, exists := c.files[lockPath]; exists {
		return "CMT_LOCK_HELD\n" + existing
	}

	c.files[lockPath] = payload

	return "CMT_LOCK_OK " + created + "\n"
}

func (c *fakeClient) dirEmpty(dir string) bool {
	for f := range c.files {
		if strings.HasPrefix(f, dir+"/") {
			return false
		}
	}

	return true
}

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

var errConnect = errors.New("connection refused")

// errFactory always fails to create a client.
type errFactory struct{}

func (errFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return nil, errConnect
}

// errRunClient connects but every RunCommand fails (e.g. SSH command error).
type errRunClient struct{ fakeClient }

func (errRunClient) RunCommand(context.Context, string, string) (string, error) {
	return "", errConnect
}

type errRunFactory struct{}

func (errRunFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return &errRunClient{fakeClient{files: map[string]string{}}}, nil
}

func checkConnErr(t *testing.T, name string, err error) {
	t.Helper()

	if !errors.Is(err, errConnect) {
		t.Errorf("%s: expected errConnect, got %v", name, err)
	}
}

func TestConnectErrorsPropagate(t *testing.T) {
	t.Parallel()

	locker := lock.NewRemote(errFactory{})
	target := testTarget()

	_, acquireErr := locker.Acquire(context.Background(), target, "plan", true)
	checkConnErr(t, "Acquire", acquireErr)

	checkConnErr(t, "Release", locker.Release(context.Background(), target, "id"))

	_, readErr := locker.Read(context.Background(), target)
	checkConnErr(t, "Read", readErr)

	checkConnErr(t, "ForceUnlock", locker.ForceUnlock(context.Background(), target))
	checkConnErr(t, "ForceUnlockWithID", locker.ForceUnlockWithID(context.Background(), target, "id"))

	_, isLockedErr := locker.IsLocked(context.Background(), target)
	checkConnErr(t, "IsLocked", isLockedErr)
}

func TestRunCommandErrorsPropagate(t *testing.T) {
	t.Parallel()

	locker := lock.NewRemote(errRunFactory{})
	target := testTarget()

	_, acquireErr := locker.Acquire(context.Background(), target, "plan", true)
	checkConnErr(t, "Acquire", acquireErr)

	_, isLockedErr := locker.IsLocked(context.Background(), target)
	checkConnErr(t, "IsLocked", isLockedErr)

	checkConnErr(t, "ForceUnlock", locker.ForceUnlock(context.Background(), target))

	// A read/SSH failure must NOT be reported as "lock not found", so Release
	// surfaces the error instead of silently treating the lock as gone.
	_, readErr := locker.Read(context.Background(), target)
	checkConnErr(t, "Read", readErr)

	if errors.Is(readErr, lock.ErrLockNotFound) {
		t.Error("Read: SSH error must not be reported as ErrLockNotFound")
	}

	releaseErr := locker.Release(context.Background(), target, "id")
	checkConnErr(t, "Release", releaseErr)
}

// existsUnreadableClient reports the lock exists but cannot read it (e.g.
// permission denied on an existing file).
type existsUnreadableClient struct{ fakeClient }

func (existsUnreadableClient) RunCommand(_ context.Context, _, command string) (string, error) {
	if strings.HasPrefix(command, "if [ -e ") {
		return "Y\n", nil
	}

	return "", nil
}

func (existsUnreadableClient) ReadFile(context.Context, string) ([]byte, error) {
	return nil, errNotSupported
}

type existsUnreadableFactory struct{}

func (existsUnreadableFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return &existsUnreadableClient{fakeClient{files: map[string]string{}, dirs: map[string]bool{}}}, nil
}

func TestReadExistsButUnreadable(t *testing.T) {
	t.Parallel()

	locker := lock.NewRemote(existsUnreadableFactory{})

	_, err := locker.Read(context.Background(), testTarget())
	if err == nil {
		t.Fatal("expected error reading an existing but unreadable lock")
	}

	if errors.Is(err, lock.ErrLockNotFound) {
		t.Error("an unreadable existing lock must not be reported as ErrLockNotFound")
	}
}

// garbageAcquireClient returns output the acquire parser does not recognise.
type garbageAcquireClient struct{ fakeClient }

func (garbageAcquireClient) RunCommand(_ context.Context, _, _ string) (string, error) {
	return "unexpected output\n", nil
}

type garbageAcquireFactory struct{}

func (garbageAcquireFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return &garbageAcquireClient{fakeClient{files: map[string]string{}, dirs: map[string]bool{}}}, nil
}

func TestAcquireUnexpectedOutputIsNotLocked(t *testing.T) {
	t.Parallel()

	locker := lock.NewRemote(garbageAcquireFactory{})

	_, err := locker.Acquire(context.Background(), testTarget(), "apply", true)
	if err == nil {
		t.Fatal("expected error for unexpected acquire output")
	}

	if errors.Is(err, lock.ErrLocked) {
		t.Error("unexpected acquire output must not be reported as ErrLocked")
	}
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

	info, err := locker.Acquire(context.Background(), target, "plan", true)
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

	err = locker.Release(context.Background(), target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing lock: %v", err)
	}
}

func TestAcquireAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = locker.Acquire(context.Background(), target, "plan", true)
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestReleaseNotOwned(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.Release(context.Background(), target, "wrong-id")
	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}
}

func TestForceUnlock(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlock(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error force-unlocking: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected lock to be released after force-unlock")
	}
}

func TestForceUnlockNoLock(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()

	err := locker.ForceUnlock(context.Background(), testTarget())
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRead(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	read, err := locker.Read(context.Background(), target)
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

	_, err := locker.Read(context.Background(), testTarget())
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestIsLocked(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected target to be unlocked initially")
	}

	_, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ = locker.IsLocked(context.Background(), target)
	if !locked {
		t.Error("expected target to be locked after acquire")
	}
}

func TestAcquireCorruptedLockFile(t *testing.T) {
	t.Parallel()

	locker, client := newTestLocker()
	target := testTarget()

	client.files[target.LockPath] = "invalid-json"

	_, err := locker.Acquire(context.Background(), target, "plan", true)
	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked wrapping, got %v", err)
	}
}

func TestForceUnlockWithIDSuccess(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlockWithID(context.Background(), target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected lock to be released after ForceUnlockWithID")
	}
}

func TestForceUnlockWithIDMismatch(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.ForceUnlockWithID(context.Background(), target, "wrong-id")
	if !errors.Is(err, lock.ErrLockIDMismatch) {
		t.Errorf("expected ErrLockIDMismatch, got %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if !locked {
		t.Error("expected lock to remain after ID mismatch")
	}
}

func TestReleaseAlreadyGone(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(context.Background(), target, "plan", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = locker.ForceUnlock(context.Background(), target)

	err = locker.Release(context.Background(), target, info.ID)
	if err != nil {
		t.Errorf("expected no error when releasing already-gone lock, got %v", err)
	}
}

func TestAcquireSkipsWhenDirAbsent(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	// ensureDir=false (plan) and the project dir does not exist => skipped.
	info, err := locker.Acquire(context.Background(), target, "plan", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info != nil {
		t.Errorf("expected nil info (skipped), got %+v", info)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected no lock file created when project dir is absent")
	}
}

func TestAcquireReportsCreatedDir(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	// First apply on an absent dir reports CreatedDir.
	info, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.CreatedDir {
		t.Error("expected CreatedDir=true when the project dir was absent")
	}

	err = locker.Release(context.Background(), target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing: %v", err)
	}

	// The dir still exists (not yet rolled back); a second apply reports false.
	info, err = locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.CreatedDir {
		t.Error("expected CreatedDir=false when the project dir already existed")
	}
}

func TestRemoveEmptyDir(t *testing.T) {
	t.Parallel()

	locker, client := newTestLocker()
	target := testTarget()

	info, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !info.CreatedDir {
		t.Fatal("expected CreatedDir=true")
	}

	err = locker.Release(context.Background(), target, info.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing: %v", err)
	}

	// Lock removed and dir empty => RemoveEmptyDir rolls the dir back.
	err = locker.RemoveEmptyDir(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error removing empty dir: %v", err)
	}

	if client.dirs[target.RemoteDir] {
		t.Error("expected empty project dir to be removed")
	}
}

func TestRemoveEmptyDirKeepsNonEmpty(t *testing.T) {
	t.Parallel()

	locker, client := newTestLocker()
	target := testTarget()

	_, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate apply having written a file into the project dir.
	client.files[target.RemoteDir+"/compose.yml"] = "services: {}"

	err = locker.RemoveEmptyDir(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !client.dirs[target.RemoteDir] {
		t.Error("expected non-empty project dir to be kept")
	}
}

func TestAcquireWithoutEnsureDirWhenDirExists(t *testing.T) {
	t.Parallel()

	locker, _ := newTestLocker()
	target := testTarget()

	// Create the dir via an apply-style acquire, release, then plan-style acquire.
	first, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = locker.Release(context.Background(), target, first.ID)
	if err != nil {
		t.Fatalf("unexpected error releasing: %v", err)
	}

	info, err := locker.Acquire(context.Background(), target, "plan", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info == nil {
		t.Fatal("expected lock to be acquired when project dir exists")
	}
}
