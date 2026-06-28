package cmd

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"
)

var (
	errNoSuchFile   = errors.New("no such file")
	errNotSupported = errors.New("not supported")
)

// fakeClient is an in-memory remote.RemoteClient emulating the lock script
// behaviour (atomic create via `set -C`, cat, rm -f, `[ -e ]`).
type fakeClient struct {
	files     map[string]string
	dirs      map[string]bool
	removeErr error
	runErr    error
	statErr   error
}

var _ remote.RemoteClient = (*fakeClient)(nil)

func (c *fakeClient) RunCommand(_ context.Context, _ string, command string) (string, error) {
	if c.runErr != nil {
		return "", c.runErr
	}

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
	if c.removeErr != nil {
		return c.removeErr
	}

	delete(c.files, remotePath)

	return nil
}

func (c *fakeClient) Close() error                                    { return nil }
func (c *fakeClient) WriteFile(context.Context, string, []byte) error { return nil }
func (c *fakeClient) MkdirAll(context.Context, string) error          { return nil }
func (c *fakeClient) Stat(context.Context, string) (fs.FileInfo, error) {
	if c.statErr != nil {
		return nil, c.statErr
	}

	return nil, errNotSupported
}
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

	release, err := acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, target := range targets {
		locked, _ := locker.IsLocked(context.Background(), target)
		if !locked {
			t.Errorf("expected %s to be locked", target.Project)
		}
	}

	_ = release()

	for _, target := range targets {
		locked, _ := locker.IsLocked(context.Background(), target)
		if locked {
			t.Errorf("expected %s to be unlocked after release", target.Project)
		}
	}
}

func TestAcquireRemoteLocksRollsBackCreatedEmptyDir(t *testing.T) {
	t.Parallel()

	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
	locker := lock.NewRemote(fakeFactory{client: client})
	targets := lockTargets("grafana")

	var buf strings.Builder

	// apply-style acquire (ensureDir=true) creates the project dir.
	release, err := acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !client.dirs["/opt/compose/grafana"] {
		t.Fatal("expected project dir to be created on acquire")
	}

	// Release with nothing written => the created empty dir is rolled back.
	_ = release()

	if client.dirs["/opt/compose/grafana"] {
		t.Error("expected created empty dir to be rolled back on release")
	}
}

func TestAcquireRemoteLocksKeepsDirWithFiles(t *testing.T) {
	t.Parallel()

	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
	locker := lock.NewRemote(fakeFactory{client: client})
	targets := lockTargets("grafana")

	var buf strings.Builder

	release, err := acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate apply writing a file before release.
	client.files["/opt/compose/grafana/compose.yml"] = "services: {}"

	_ = release()

	if !client.dirs["/opt/compose/grafana"] {
		t.Error("expected dir with files to be kept after release")
	}
}

func TestAcquireRemoteLocksReleaseSurfacesError(t *testing.T) {
	t.Parallel()

	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool), removeErr: errNotSupported}
	locker := lock.NewRemote(fakeFactory{client: client})
	targets := lockTargets("grafana")

	var buf strings.Builder

	release, err := acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Release fails to remove the lock file => the failure must be reported,
	// not swallowed.
	releaseErr := release()
	if releaseErr == nil {
		t.Error("expected release() to surface the lock-release failure")
	}
}

func TestAcquireRemoteLocksAlreadyLocked(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	targets := lockTargets("grafana")

	_, err := locker.Acquire(context.Background(), targets[0], "existing-op", true)
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring lock: %v", err)
	}

	var buf strings.Builder

	release, err := acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err == nil {
		_ = release()

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
	_, err := locker.Acquire(context.Background(), targets[1], "existing-op", true)
	if err != nil {
		t.Fatalf("unexpected error pre-acquiring second target: %v", err)
	}

	var buf strings.Builder

	_, err = acquireRemoteLocks(context.Background(), locker, targets, &buf)
	if err == nil {
		t.Fatal("expected error when second target is already locked")
	}

	locked, _ := locker.IsLocked(context.Background(), targets[0])
	if locked {
		t.Error("expected first target to be released after partial failure")
	}
}

func TestAcquireRemoteLocksEmpty(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	var buf strings.Builder

	release, err := acquireRemoteLocks(context.Background(), locker, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error for empty target list: %v", err)
	}

	_ = release()
}

func TestRunForceUnlockWithLockerNotFound(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	err := runForceUnlockWithLocker(context.Background(), locker, lockTargets("grafana")[0], false)
	if !errors.Is(err, lock.ErrLockNotFound) {
		t.Errorf("expected ErrLockNotFound, got %v", err)
	}
}

func TestRunForceUnlockWithLockerSuccess(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	err = runForceUnlockWithLocker(context.Background(), locker, target, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected lock to be released after force-unlock")
	}
}

func TestRunForceUnlockWithLockerCancelConfirm(t *testing.T) { //nolint:paralleltest
	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(context.Background(), target, "plan", true)
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

	err = runForceUnlockWithLocker(context.Background(), locker, target, false)
	if err != nil {
		t.Fatalf("unexpected error for cancelled force-unlock: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if !locked {
		t.Error("expected target to still be locked after cancelled force-unlock")
	}
}

func TestForceUnlockManyReleasesOnlyLocked(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()
	targets := lockTargets("grafana", "n8n", "vault")

	// Lock two of the three candidates.
	for _, i := range []int{0, 2} {
		_, err := locker.Acquire(context.Background(), targets[i], "apply", true)
		if err != nil {
			t.Fatalf("unexpected error acquiring lock: %v", err)
		}
	}

	// force=true skips confirmation; n8n is untouched because it was never locked.
	err := forceUnlockMany(context.Background(), locker, targets, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, target := range targets {
		locked, _ := locker.IsLocked(context.Background(), target)
		if locked {
			t.Errorf("expected %s to be unlocked", target.Project)
		}
	}
}

func TestForceUnlockManyNoLocks(t *testing.T) {
	t.Parallel()

	locker := newTestLocker()

	// No targets are locked, so this is a no-op success (no confirmation needed).
	err := forceUnlockMany(context.Background(), locker, lockTargets("grafana", "n8n"), false)
	if err != nil {
		t.Fatalf("unexpected error for no-lock batch: %v", err)
	}
}

func TestValidateForceUnlockArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		args    []string
		opts    forceUnlockOptions
		wantErr error
	}{
		{"single ok", []string{"host1", "grafana"}, forceUnlockOptions{}, nil},
		{"single missing arg", []string{"host1"}, forceUnlockOptions{}, errForceUnlockNeedsTwoArgs},
		{"all ok", nil, forceUnlockOptions{all: true}, nil},
		{"all with args", []string{"host1"}, forceUnlockOptions{all: true}, errForceUnlockAllNoArgs},
		{
			"all with filters", nil,
			forceUnlockOptions{all: true, hostFilter: []string{"host1"}}, nil,
		},
		{
			"filters without all", []string{"host1", "grafana"},
			forceUnlockOptions{projectFilter: []string{"grafana"}}, errForceUnlockFiltersNeedAll,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateForceUnlockArgs(tc.args, tc.opts)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("validateForceUnlockArgs() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestForceUnlockManyUnreadableForceRemoves(t *testing.T) {
	t.Parallel()

	// An existing-but-corrupt lock can't be read; with --force it must still be
	// removed, mirroring the single-target force path.
	target := lockTargets("grafana")[0]
	client := &fakeClient{files: map[string]string{target.LockPath: "{not valid json"}}
	locker := lock.NewRemote(fakeFactory{client: client})

	err := forceUnlockMany(context.Background(), locker, []lock.Target{target}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if locked {
		t.Error("expected unreadable lock to be removed with --force")
	}
}

func TestForceUnlockManyUnreadableWithoutForceErrors(t *testing.T) { //nolint:paralleltest
	// Without --force an unreadable lock can't be safely removed: it must be left
	// in place and reported as an error.
	target := lockTargets("grafana")[0]
	client := &fakeClient{files: map[string]string{target.LockPath: "{not valid json"}}
	locker := lock.NewRemote(fakeFactory{client: client})

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("unexpected error creating pipe: %v", pipeErr)
	}

	_, _ = w.WriteString("y\n")
	_ = w.Close()

	oldStdin := os.Stdin
	os.Stdin = r

	t.Cleanup(func() { os.Stdin = oldStdin; _ = r.Close() })

	err := forceUnlockMany(context.Background(), locker, []lock.Target{target}, false)
	if !errors.Is(err, errUnreadableLockNeedsForce) {
		t.Errorf("expected errUnreadableLockNeedsForce, got %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if !locked {
		t.Error("expected unreadable lock to be left in place without --force")
	}
}

func TestForceUnlockManyReleaseErrorSurfaced(t *testing.T) {
	t.Parallel()

	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
	locker := lock.NewRemote(fakeFactory{client: client})
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(context.Background(), target, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	// Removal now fails: the batch must surface the release error.
	client.removeErr = errNotSupported

	err = forceUnlockMany(context.Background(), locker, []lock.Target{target}, true)
	if err == nil {
		t.Error("expected forceUnlockMany to surface the release failure")
	}
}

func TestForceUnlockManyMultipleReleaseErrors(t *testing.T) {
	t.Parallel()

	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
	locker := lock.NewRemote(fakeFactory{client: client})
	targets := lockTargets("grafana", "n8n")

	for _, target := range targets {
		_, err := locker.Acquire(context.Background(), target, "apply", true)
		if err != nil {
			t.Fatalf("unexpected error acquiring lock: %v", err)
		}
	}

	// Both removals fail: only the first error is surfaced, the rest are warnings.
	client.removeErr = errNotSupported

	err := forceUnlockMany(context.Background(), locker, targets, true)
	if !errors.Is(err, errNotSupported) {
		t.Errorf("expected first release error to surface, got %v", err)
	}
}

func TestForceUnlockManyCancelConfirm(t *testing.T) { //nolint:paralleltest
	locker := newTestLocker()
	target := lockTargets("grafana")[0]

	_, err := locker.Acquire(context.Background(), target, "plan", true)
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

	err = forceUnlockMany(context.Background(), locker, []lock.Target{target}, false)
	if err != nil {
		t.Fatalf("unexpected error for cancelled batch: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), target)
	if !locked {
		t.Error("expected target to still be locked after cancelled force-unlock")
	}
}

// cmdStubResolver satisfies config.SSHConfigResolver without invoking ssh.
type cmdStubResolver struct{}

func (cmdStubResolver) Resolve(_ context.Context, entry *config.HostEntry, _, _ string) error {
	if entry.Port == 0 {
		entry.Port = 22
	}

	return nil
}

func writeCmdLockRepo(t *testing.T, projects ...string) *config.CmtConfig {
	t.Helper()

	base := t.TempDir()

	for _, p := range projects {
		err := os.MkdirAll(filepath.Join(base, "projects", p), 0o750)
		if err != nil {
			t.Fatalf("unexpected error creating project %q: %v", p, err)
		}
	}

	return &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/opt/compose"},
		Hosts: []config.HostEntry{
			{Name: "host1", Host: "host1-alias", User: "deploy"},
		},
	}
}

func TestRunForceUnlockAllReleasesLocked(t *testing.T) {
	t.Parallel()

	cfg := writeCmdLockRepo(t, "grafana", "n8n")
	client := &fakeClient{files: make(map[string]string), dirs: make(map[string]bool)}
	locker := lock.NewRemote(fakeFactory{client: client})

	// Lock grafana only; n8n stays unlocked and must be left untouched.
	graf := lock.Target{
		Host:      cfg.Hosts[0],
		Project:   "grafana",
		RemoteDir: "/opt/compose/grafana",
		LockPath:  "/opt/compose/grafana/.cmt.lock",
	}

	_, err := locker.Acquire(context.Background(), graf, "apply", true)
	if err != nil {
		t.Fatalf("unexpected error acquiring lock: %v", err)
	}

	deps := syncer.PlanDependencies{SSHResolver: cmdStubResolver{}}
	opts := forceUnlockOptions{force: true, all: true}

	err = runForceUnlockAll(context.Background(), locker, cfg, opts, deps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	locked, _ := locker.IsLocked(context.Background(), graf)
	if locked {
		t.Error("expected grafana to be unlocked after --all force-unlock")
	}
}

func TestRunForceUnlockDispatchesAll(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	// --all narrowed to a non-existent project takes the batch branch and fails
	// at target resolution, before any remote (SSH) work is attempted.
	opts := forceUnlockOptions{force: true, all: true, projectFilter: []string{"nonexistent"}}

	err := runForceUnlock(context.Background(), configPath, nil, opts)
	if err == nil {
		t.Error("expected error when --all matches no project")
	}
}

func TestRunForceUnlockRejectsArgs(t *testing.T) {
	t.Parallel()

	configPath := writeTestRepo(t)

	// --all with a stray positional arg is rejected before config work.
	err := runForceUnlock(context.Background(), configPath, []string{"host1"}, forceUnlockOptions{all: true})
	if !errors.Is(err, errForceUnlockAllNoArgs) {
		t.Errorf("expected errForceUnlockAllNoArgs, got %v", err)
	}
}

func TestRunForceUnlockAllResolveError(t *testing.T) {
	t.Parallel()

	cfg := writeCmdLockRepo(t, "grafana")
	locker := newTestLocker()
	deps := syncer.PlanDependencies{SSHResolver: cmdStubResolver{}}
	opts := forceUnlockOptions{force: true, all: true, projectFilter: []string{"nonexistent"}}

	// A non-existent project filter resolves to no targets -> error returned
	// before any locker work.
	err := runForceUnlockAll(context.Background(), locker, cfg, opts, deps)
	if err == nil {
		t.Error("expected error when no targets match the filter")
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

		if !confirmForceUnlock(context.Background(), "host1/grafana") {
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

	if confirmForceUnlock(context.Background(), "host1/grafana") {
		t.Error("expected confirmForceUnlock to return false for 'n'")
	}
}
