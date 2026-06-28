package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

// lockReleaseTimeout bounds each lock-release attempt during cleanup. The
// cleanup context ignores the caller's cancellation (so Ctrl+C still releases
// locks), which also drops its deadline — and SSH ConnectTimeout only bounds the
// connect phase, not a host that connects then stops responding. Without this
// cap such a host would make release hang forever, unkillable by further signals.
const lockReleaseTimeout = 30 * time.Second

type acquiredLock struct {
	target     lock.Target
	lockID     string
	createdDir bool
}

// acquireRemoteLocks takes the locks for all targets. The returned release
// function removes them and reports the first release failure — a leaked remote
// lock blocks later operations, so callers must surface it rather than ignore it.
//
// Only apply acquires locks (plan is read-only and never locks), so the project
// directory is always created when missing.
func acquireRemoteLocks(
	ctx context.Context,
	locker *lock.RemoteLocker,
	targets []lock.Target,
	w io.Writer,
) (func() error, error) {
	var acquired []acquiredLock

	releaseFn := func() error {
		var firstErr error

		for _, a := range acquired {
			err := releaseAcquiredLock(ctx, locker, a)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to release lock %s/%s: %v\n",
					a.target.Host.Name, a.target.Project, err)

				if firstErr == nil {
					firstErr = err
				}
			}
		}

		return firstErr
	}

	for _, target := range targets {
		info, err := locker.Acquire(ctx, target, "apply", true)
		if err != nil {
			_ = releaseFn()

			return func() error { return nil }, err
		}

		if info == nil {
			// Defensive: Acquire only returns nil info when the directory is
			// missing and not created, which cannot happen here (ensureDir).
			continue
		}

		_, _ = fmt.Fprintf(w, "Lock acquired: %s/%s\n", target.Host.Name, target.Project)
		acquired = append(acquired, acquiredLock{target: target, lockID: info.ID, createdDir: info.CreatedDir})
	}

	return releaseFn, nil
}

// releaseAcquiredLock removes a single acquired lock during cleanup. It derives
// a context that ignores the caller's cancellation (so Ctrl+C still releases the
// lock) but caps the attempt at lockReleaseTimeout so an unresponsive host can't
// block cleanup indefinitely.
func releaseAcquiredLock(ctx context.Context, locker *lock.RemoteLocker, acquired acquiredLock) error {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lockReleaseTimeout)
	defer cancel()

	err := locker.Release(releaseCtx, acquired.target, acquired.lockID)

	// Roll back a directory that acquisition created if the operation left it
	// empty (e.g. apply cancelled before writing anything).
	if acquired.createdDir {
		_ = locker.RemoveEmptyDir(releaseCtx, acquired.target)
	}

	return err
}
