package lock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
)

const (
	// LockFileName is the per-project lock file created inside each project's
	// remote directory (e.g. /opt/compose/<project>/.cmt.lock).
	LockFileName = ".cmt.lock"

	lockIDBytes = 16
	markerOK    = "CMT_LOCK_OK"
	markerHeld  = "CMT_LOCK_HELD"
	markerNoDir = "CMT_LOCK_NODIR"
)

const lockedInfoFmt = "%w for %s:" +
	"\n  ID:        %s" +
	"\n  Operation: %s" +
	"\n  Who:       %s" +
	"\n  Created:   %s" +
	"\n\nTo force-unlock: cmt force-unlock %s %s"

var (
	ErrLocked                  = errors.New("host is locked by another operation")
	ErrLockNotFound            = errors.New("lock not found")
	ErrLockIDMismatch          = errors.New("lock ID mismatch")
	errUnexpectedAcquireOutput = errors.New("unexpected lock acquire output")
)

// Info contains metadata about an active lock.
type Info struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Who       string    `json:"who"`
	Created   time.Time `json:"created"`
	Path      string    `json:"path"`

	// CreatedDir reports whether acquiring the lock created the project's remote
	// directory (apply only). It is not persisted; callers use it to roll back
	// the directory on release if nothing was written. json:"-"
	CreatedDir bool `json:"-"`
}

// Target identifies a per-project lock living on a remote host.
type Target struct {
	Host      config.HostEntry
	Project   string
	RemoteDir string
	LockPath  string
}

func (t Target) lockPath() string {
	if t.LockPath != "" {
		return t.LockPath
	}

	return path.Join(t.RemoteDir, LockFileName)
}

func (t Target) label() string {
	return t.Host.Name + "/" + t.Project
}

// RemoteLocker manages per-project lock files on remote hosts over SSH.
type RemoteLocker struct {
	factory remote.ClientFactory
}

// NewRemote returns a RemoteLocker that connects through the given factory.
func NewRemote(factory remote.ClientFactory) *RemoteLocker {
	return &RemoteLocker{factory: factory}
}

// Acquire atomically creates the lock file for target on the remote host.
// Returns ErrLocked if a lock is already held.
//
// When ensureDir is true (apply), the project's remote directory is created if
// missing so the lock can be placed before any files are synced. When false
// (plan), no directory is created: if the project directory does not exist yet
// there is no remote state to protect, so Acquire returns (nil, nil) to signal
// the lock was skipped.
func (l *RemoteLocker) Acquire(target Target, operation string, ensureDir bool) (*Info, error) {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	info := &Info{
		ID:         generateID(),
		Operation:  operation,
		Who:        whoString(),
		Created:    time.Now().UTC(),
		Path:       target.lockPath(),
		CreatedDir: false,
	}

	data, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("encoding lock info: %w", err)
	}

	out, err := client.RunCommand("", buildAcquireScript(target, data, ensureDir))
	if err != nil {
		return nil, fmt.Errorf("acquiring lock for %s: %w", target.label(), err)
	}

	return parseAcquireOutput(target, info, out)
}

func buildAcquireScript(target Target, jsonData []byte, ensureDir bool) string {
	dir := shellQuote(target.RemoteDir)
	lockFile := shellQuote(target.lockPath())
	payload := shellQuote(string(jsonData))

	// Attempt the atomic create. On success print "<markerOK> <created>". If it
	// fails ONLY because the lock already exists, print <markerHeld>; any other
	// failure (permission, bad parent path) exits non-zero so the caller sees
	// the real error instead of treating it as a conflict.
	takeLock := fmt.Sprintf(
		"if ( set -C; printf '%%s' %s > %s ) 2>/dev/null; then echo %s $created; "+
			"elif [ -e %s ]; then echo %s; cat %s 2>/dev/null || true; else exit 1; fi",
		payload, lockFile, markerOK, lockFile, markerHeld, lockFile,
	)

	if ensureDir {
		// apply: create the project directory if missing (a mkdir failure exits
		// non-zero), recording whether we created it so release can roll it back.
		return fmt.Sprintf(
			"created=0; if [ ! -d %s ]; then mkdir -p %s || exit 1; created=1; fi; %s",
			dir, dir, takeLock,
		)
	}

	// plan: never create directories. If the project directory is absent there
	// is nothing to lock yet.
	return fmt.Sprintf("created=0; if [ -d %s ]; then %s; else echo %s; fi", dir, takeLock, markerNoDir)
}

func parseAcquireOutput(target Target, info *Info, out string) (*Info, error) {
	trimmed := strings.TrimSpace(out)

	switch {
	case strings.HasPrefix(trimmed, markerOK):
		fields := strings.Fields(trimmed)
		info.CreatedDir = len(fields) > 1 && fields[1] == "1"

		return info, nil
	case strings.HasPrefix(trimmed, markerNoDir):
		// Project not deployed yet; nothing to lock.
		return nil, nil //nolint:nilnil
	case strings.HasPrefix(trimmed, markerHeld):
		holderJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, markerHeld))

		return nil, lockedError(target, holderJSON)
	default:
		return nil, fmt.Errorf("%w for %s: %q", errUnexpectedAcquireOutput, target.label(), trimmed)
	}
}

func lockedError(target Target, holderJSON string) error {
	var existing Info

	if holderJSON != "" && json.Unmarshal([]byte(holderJSON), &existing) == nil {
		return fmt.Errorf(
			lockedInfoFmt,
			ErrLocked, target.label(),
			existing.ID, existing.Operation, existing.Who,
			existing.Created.Format(time.RFC3339),
			target.Host.Name, target.Project,
		)
	}

	return fmt.Errorf(
		"%w for %s\n\nTo force-unlock: cmt force-unlock %s %s",
		ErrLocked, target.label(), target.Host.Name, target.Project,
	)
}

// Release removes the lock for target when the ID matches the one in the file.
func (l *RemoteLocker) Release(target Target, lockID string) error {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	existing, err := readWithClient(client, target)
	if err != nil {
		if errors.Is(err, ErrLockNotFound) {
			return nil
		}

		return fmt.Errorf("reading lock for %s: %w", target.label(), err)
	}

	if existing.ID != lockID {
		return fmt.Errorf("%w for %s: own %s but file has %s", ErrLockIDMismatch, target.label(), lockID, existing.ID)
	}

	err = client.Remove(target.lockPath())
	if err != nil {
		return fmt.Errorf("removing lock for %s: %w", target.label(), err)
	}

	return nil
}

// Read returns the current lock info for target. Returns ErrLockNotFound if none exists.
func (l *RemoteLocker) Read(target Target) (*Info, error) {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	return readWithClient(client, target)
}

func readWithClient(client remote.RemoteClient, target Target) (*Info, error) {
	// Distinguish a genuinely absent lock from a read failure (SSH/permission):
	// only the former is ErrLockNotFound, so Release/force-unlock don't treat a
	// transient error as "no lock" and silently leave it behind.
	exists, err := existsWithClient(client, target)
	if err != nil {
		return nil, fmt.Errorf("checking lock for %s: %w", target.label(), err)
	}

	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrLockNotFound, target.label())
	}

	data, err := client.ReadFile(target.lockPath())
	if err != nil {
		return nil, fmt.Errorf("reading lock for %s: %w", target.label(), err)
	}

	var info Info

	err = json.Unmarshal(data, &info)
	if err != nil {
		return nil, fmt.Errorf("parsing lock file for %s: %w", target.label(), err)
	}

	return &info, nil
}

// ForceUnlock removes the lock for target regardless of who holds it.
func (l *RemoteLocker) ForceUnlock(target Target) error {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	exists, err := existsWithClient(client, target)
	if err != nil {
		return fmt.Errorf("checking lock for %s: %w", target.label(), err)
	}

	if !exists {
		return fmt.Errorf("%w: %s", ErrLockNotFound, target.label())
	}

	err = client.Remove(target.lockPath())
	if err != nil {
		return fmt.Errorf("removing lock for %s: %w", target.label(), err)
	}

	return nil
}

// ForceUnlockWithID removes the lock for target only when the current lock ID
// still matches expectedID. Returns ErrLockIDMismatch if the lock was replaced
// between the time it was displayed and the time the user confirmed.
func (l *RemoteLocker) ForceUnlockWithID(target Target, expectedID string) error {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	current, err := readWithClient(client, target)
	if err != nil {
		return err
	}

	if current.ID != expectedID {
		return fmt.Errorf("%w for %s: expected %s but found %s", ErrLockIDMismatch, target.label(), expectedID, current.ID)
	}

	err = client.Remove(target.lockPath())
	if err != nil {
		return fmt.Errorf("removing lock for %s: %w", target.label(), err)
	}

	return nil
}

// RemoveEmptyDir removes target's project directory if it is empty. It is used
// to roll back a directory that lock acquisition created when an apply ends
// without writing anything. A non-empty directory (a real deployment) is left
// untouched.
func (l *RemoteLocker) RemoveEmptyDir(target Target) error {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	// rmdir only succeeds on an empty directory; ignore failure otherwise.
	_, err = client.RunCommand("", "rmdir "+shellQuote(target.RemoteDir)+" 2>/dev/null || true")
	if err != nil {
		return fmt.Errorf("removing empty dir for %s: %w", target.label(), err)
	}

	return nil
}

// IsLocked reports whether a lock file exists for target.
func (l *RemoteLocker) IsLocked(target Target) (bool, error) {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return false, fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	return existsWithClient(client, target)
}

func existsWithClient(client remote.RemoteClient, target Target) (bool, error) {
	cmd := fmt.Sprintf("if [ -e %s ]; then echo Y; else echo N; fi", shellQuote(target.lockPath()))

	out, err := client.RunCommand("", cmd)
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(out) == "Y", nil
}

func generateID() string {
	b := make([]byte, lockIDBytes)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

func whoString() string {
	hostname, _ := os.Hostname()
	user := os.Getenv("USER")

	if user == "" {
		user = os.Getenv("USERNAME")
	}

	if user == "" {
		user = "unknown"
	}

	pid := strconv.Itoa(os.Getpid())

	return fmt.Sprintf("%s@%s (PID %s)", user, hostname, pid)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
