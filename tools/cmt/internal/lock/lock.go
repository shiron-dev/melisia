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

var ErrLocked = errors.New("host is locked by another operation")

type Info struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Who       string    `json:"who"`
	Created   time.Time `json:"created"`
	Path      string    `json:"path"`
}

func Dir() string {
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

func path(hostName string) string {
	return filepath.Join(Dir(), hostName+".lock")
}

func Acquire(hostName, operation string) (*Info, error) {
	lockDir := Dir()

	err := os.MkdirAll(lockDir, 0o700)
	if err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	lockPath := path(hostName)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			existing, readErr := Read(hostName)
			if readErr != nil {
				return nil, fmt.Errorf(
					"%w for host %q (lock file exists but could not be read: %v)\n\nTo force-unlock: cmt force-unlock %s",
					ErrLocked, hostName, readErr, hostName,
				)
			}

			return nil, fmt.Errorf(
				"%w for host %q:\n  ID:        %s\n  Operation: %s\n  Who:       %s\n  Created:   %s\n\nTo force-unlock: cmt force-unlock %s",
				ErrLocked, hostName,
				existing.ID, existing.Operation, existing.Who,
				existing.Created.Format(time.RFC3339),
				hostName,
			)
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

func Release(hostName, lockID string) error {
	existing, err := Read(hostName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("reading lock for %q: %w", hostName, err)
	}

	if existing.ID != lockID {
		return fmt.Errorf("lock ID mismatch for %q: own %s but file has %s", hostName, lockID, existing.ID)
	}

	return os.Remove(path(hostName))
}

func Read(hostName string) (*Info, error) {
	data, err := os.ReadFile(path(hostName))
	if err != nil {
		return nil, err
	}

	var info Info

	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing lock file for %q: %w", hostName, err)
	}

	return &info, nil
}

func ForceUnlock(hostName string) error {
	lockPath := path(hostName)

	err := os.Remove(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no lock found for host %q", hostName)
		}

		return fmt.Errorf("removing lock for %q: %w", hostName, err)
	}

	return nil
}

func generateID() string {
	b := make([]byte, 16)
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

func IsLocked(hostName string) bool {
	_, err := os.Stat(path(hostName))

	return err == nil
}
