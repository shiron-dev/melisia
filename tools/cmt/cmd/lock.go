package cmd

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

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

	// Release runs during cleanup, which is exactly when ctx may already be
	// cancelled (Ctrl+C). Derive a context that ignores that cancellation so a
	// leaked lock still gets removed; the per-command ConnectTimeout still bounds
	// it against a host that has gone away.
	releaseCtx := context.WithoutCancel(ctx)

	releaseFn := func() error {
		var firstErr error

		for _, a := range acquired {
			err := locker.Release(releaseCtx, a.target, a.lockID)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to release lock %s/%s: %v\n",
					a.target.Host.Name, a.target.Project, err)

				if firstErr == nil {
					firstErr = err
				}
			}

			// Roll back a directory that acquisition created if the operation
			// left it empty (e.g. apply cancelled before writing anything).
			if a.createdDir {
				_ = locker.RemoveEmptyDir(releaseCtx, a.target)
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
