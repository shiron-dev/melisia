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
)

const lockedInfoFmt = "%w for %s:" +
	"\n  ID:        %s" +
	"\n  Operation: %s" +
	"\n  Who:       %s" +
	"\n  Created:   %s" +
	"\n\nTo force-unlock: cmt force-unlock %s %s"

var (
	ErrLocked         = errors.New("host is locked by another operation")
	ErrLockNotFound   = errors.New("lock not found")
	ErrLockIDMismatch = errors.New("lock ID mismatch")
)

// Info contains metadata about an active lock.
type Info struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Who       string    `json:"who"`
	Created   time.Time `json:"created"`
	Path      string    `json:"path"`
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
func (l *RemoteLocker) Acquire(target Target, operation string) (*Info, error) {
	client, err := l.factory.NewClient(target.Host)
	if err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", target.Host.Name, err)
	}

	defer func() { _ = client.Close() }()

	info := &Info{
		ID:        generateID(),
		Operation: operation,
		Who:       whoString(),
		Created:   time.Now().UTC(),
		Path:      target.lockPath(),
	}

	data, err := json.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("encoding lock info: %w", err)
	}

	out, err := client.RunCommand("", buildAcquireScript(target, data))
	if err != nil {
		return nil, fmt.Errorf("acquiring lock for %s: %w", target.label(), err)
	}

	return parseAcquireOutput(target, info, out)
}

func buildAcquireScript(target Target, jsonData []byte) string {
	dir := shellQuote(target.RemoteDir)
	lockFile := shellQuote(target.lockPath())
	payload := shellQuote(string(jsonData))

	return fmt.Sprintf(
		"if mkdir -p %s 2>/dev/null && ( set -C; printf '%%s' %s > %s ) 2>/dev/null; then "+
			"echo %s; else echo %s; cat %s 2>/dev/null || true; fi",
		dir, payload, lockFile, markerOK, markerHeld, lockFile,
	)
}

func parseAcquireOutput(target Target, info *Info, out string) (*Info, error) {
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, markerOK) {
		return info, nil
	}

	holderJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, markerHeld))

	return nil, lockedError(target, holderJSON)
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
	data, err := client.ReadFile(target.lockPath())
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrLockNotFound, target.label())
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
