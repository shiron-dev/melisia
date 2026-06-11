package lock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	lockDirPerms  = 0o700
	lockFilePerms = 0o600
	lockIDBytes   = 16
)

const lockedInfoFmt = "%w for host %q:" +
	"\n  ID:        %s" +
	"\n  Operation: %s" +
	"\n  Who:       %s" +
	"\n  Created:   %s" +
	"\n\nTo force-unlock: cmt force-unlock %s"

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

// Locker manages per-host lock files in a configured directory.
type Locker struct {
	dir string
}

// New returns a Locker that uses the XDG-standard lock directory.
func New() *Locker {
	return &Locker{dir: defaultDir()}
}

// NewWithDir returns a Locker that stores lock files in dir.
func NewWithDir(dir string) *Locker {
	return &Locker{dir: dir}
}

// Dir returns the directory where lock files are stored.
func (l *Locker) Dir() string { return l.dir }

func defaultDir() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", ".cmt", "locks")
		}

		dataHome = filepath.Join(home, ".local", "share")
	}

	return filepath.Join(dataHome, "cmt", "locks")
}

// Acquire atomically creates a lock for hostName. Returns ErrLocked if already held.
func (l *Locker) Acquire(hostName, operation string) (*Info, error) {
	err := os.MkdirAll(l.dir, lockDirPerms)
	if err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lockPath := l.filePath(hostName)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, lockFilePerms) //nolint:gosec
	if err != nil {
		if os.IsExist(err) {
			return nil, l.lockedError(hostName)
		}

		return nil, fmt.Errorf("acquiring lock for %q: %w", hostName, err)
	}

	defer func() { _ = f.Close() }()

	info := &Info{
		ID:        generateID(),
		Operation: operation,
		Who:       whoString(),
		Created:   time.Now().UTC(),
		Path:      lockPath,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		_ = os.Remove(lockPath)

		return nil, fmt.Errorf("encoding lock info: %w", err)
	}

	_, err = f.Write(data)
	if err != nil {
		_ = os.Remove(lockPath)

		return nil, fmt.Errorf("writing lock file: %w", err)
	}

	return info, nil
}

// Release removes the lock for hostName if the ID matches the one in the lock file.
func (l *Locker) Release(hostName, lockID string) error {
	existing, err := l.Read(hostName)
	if err != nil {
		if errors.Is(err, ErrLockNotFound) {
			return nil
		}

		return fmt.Errorf("reading lock for %q: %w", hostName, err)
	}

	if existing.ID != lockID {
		return fmt.Errorf("%w for host %q: own %s but file has %s", ErrLockIDMismatch, hostName, lockID, existing.ID)
	}

	return os.Remove(l.filePath(hostName))
}

// Read returns the current lock info for hostName. Returns ErrLockNotFound if none exists.
func (l *Locker) Read(hostName string) (*Info, error) {
	data, err := os.ReadFile(l.filePath(hostName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: host %q", ErrLockNotFound, hostName)
		}

		return nil, err
	}

	var info Info

	err = json.Unmarshal(data, &info)
	if err != nil {
		return nil, fmt.Errorf("parsing lock file for %q: %w", hostName, err)
	}

	return &info, nil
}

// ForceUnlock removes the lock for hostName regardless of who holds it.
func (l *Locker) ForceUnlock(hostName string) error {
	err := os.Remove(l.filePath(hostName))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: host %q", ErrLockNotFound, hostName)
		}

		return fmt.Errorf("removing lock for %q: %w", hostName, err)
	}

	return nil
}

// ForceUnlockWithID removes the lock for hostName only when the current lock ID
// still matches expectedID. Returns ErrLockIDMismatch if the lock was replaced
// between the time it was displayed and the time the user confirmed.
func (l *Locker) ForceUnlockWithID(hostName, expectedID string) error {
	current, err := l.Read(hostName)
	if err != nil {
		return err
	}

	if current.ID != expectedID {
		return fmt.Errorf("%w for host %q: expected %s but found %s", ErrLockIDMismatch, hostName, expectedID, current.ID)
	}

	return l.ForceUnlock(hostName)
}

// IsLocked reports whether a lock file exists for hostName.
func (l *Locker) IsLocked(hostName string) bool {
	_, err := os.Stat(l.filePath(hostName))

	return err == nil
}

func (l *Locker) filePath(hostName string) string {
	return filepath.Join(l.dir, hostName+".lock")
}

func (l *Locker) lockedError(hostName string) error {
	existing, readErr := l.Read(hostName)
	if readErr != nil {
		return fmt.Errorf(
			"%w for host %q (lock file exists but could not be read: %w)\n\nTo force-unlock: cmt force-unlock %s",
			ErrLocked, hostName, readErr, hostName,
		)
	}

	return fmt.Errorf(
		lockedInfoFmt,
		ErrLocked, hostName,
		existing.ID, existing.Operation, existing.Who,
		existing.Created.Format(time.RFC3339),
		hostName,
	)
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
