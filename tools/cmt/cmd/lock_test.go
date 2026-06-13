package cmd

import (
	"errors"
	"io/fs"
	"os"
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

// fakeClient is an in-memory remote.RemoteClient emulating the lock script
// behaviour (atomic create via `set -C`, cat, rm -f, `[ -e ]`).
type fakeClient struct {
	files map[string]string
}

var _ remote.RemoteClient = (*fakeClient)(nil)

func (c *fakeClient) RunCommand(_ string, command string) (string, error) {
	quoted := extractQuoted(command)

	switch {
	case strings.HasPrefix(command, "if mkdir -p"):
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

type fakeFactory struct {
	client *fakeClient
}

func (f fakeFactory) NewClient(config.HostEntry) (remote.RemoteClient, error) {
	return f.client, nil
}

func newTestLocker() *lock.RemoteLocker {
	return lock.NewRemote(fakeFactory{client: &fakeClient{files: make(map[string]string)}})
}

func lockTargets(projects ...string) []lock.Target {
	targets := make([]lock.Target, 0, len(projects))
	for _, p := range projects {
		targets = append(targets, lock.Target{
			Host:      config.HostEntry{Name: "host1"},
			Project:   p,
			RemoteDir: "/opt/compose/" + p,
			LockPath:  "/opt/compose/" + p + "/.cmt.lock",
		})
	}

	return targets
}

func TestAcquireRemoteLocksSuccess(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	targets := lockTargets("grafana", "n8n")

	var buf strings.Builder

	release, err := acquireRemoteLocks(locker, targets, "plan", &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, target := range targets {
		locked, _ := locker.IsLocked(target)
		if !locked {
			t.Errorf("expected %s to be locked", target.Project)
		}
	}

	release()

	for _, target := range targets {
		locked, _ := locker.IsLocked(target)
		if locked {
			t.Errorf("expected %s to be unlocked after release", target.Project)
		}
	}
}

func TestAcquireRemoteLocksAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	targets := lockTargets("grafana")

	_, err := locker.Acquire(targets[0], "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	var buf strings.Builder

	release, err := acquireRemoteLocks(locker, targets, "plan", &buf)
	if err == nil {
		release()
		t.Fatal("expected error when target is already locked")
	}

	if !errors.Is(err, lock.ErrLocked) {
		t.Errorf("expected ErrLocked, got %v", err)
	}
}

func TestAcquireRemoteLocksReleasesOnPartialFailure(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	targets := lockTargets("grafana", "n8n")

	// Pre-lock the second target so the second acquire fails.
	_, err := locker.Acquire(targets[1], "existing-op")
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring second target: %v", err)
	}

	var buf strings.Builder

	_, err = acquireRemoteLocks(locker, targets, "plan", &buf)
	if err == nil {
		t.Fatal("expected error when second target is already locked")
	}

	locked, _ := locker.IsLocked(targets[0])
	if locked {
		t.Error("expected first target to be released after partial failure")
	}
}

func TestAcquireRemoteLocksEmpty(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	var buf strings.Builder

	release, err := acquireRemoteLocks(locker, nil, "plan", &buf)
	if err != nil {
		t.Fatalf("unexpected error for empty target list: %v", err)
	}

	release()
}

func TestRunForceUnlockWithLockerNotFound(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	err := runForceUnlockWithLocker(locker, lockTargets("grafana")[0], false)
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRunForceUnlockWithLockerSuccess(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(target, "apply")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	err = runForceUnlockWithLocker(locker, target, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if locked {
		t.Error("expected lock to be released after force-unlock")
	}
}

func TestRunForceUnlockWithLockerCancelConfirm(t *testing.T) { //nolint:paralleltest
	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(target, "plan")
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("unexpected error creating pipe: %v", pipeErr)
	}

	_, _ = w.WriteString("n\n")
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })

	err = runForceUnlockWithLocker(locker, target, false)
	if err != nil {
		t.Fatalf("unexpected error for cancelled force-unlock: %v", err)
	}

	locked, _ := locker.IsLocked(target)
	if !locked {
		t.Error("expected target to still be locked after cancelled force-unlock")
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

		if !confirmForceUnlock("host1/grafana") {
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

	if confirmForceUnlock("host1/grafana") {
		t.Error("expected confirmForceUnlock to return false for 'n'")
	}
}
