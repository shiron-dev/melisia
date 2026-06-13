package cmd

import (
	"fmt"
	"io"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

type acquiredLock struct {
	target lock.Target
	lockID string
}

func acquireRemoteLocks(
	locker *lock.RemoteLocker,
	targets []lock.Target,
	operation string,
	ensureDir bool,
	w io.Writer,
) (func(), error) {
	var acquired []acquiredLock

	releaseFn := func() {
		for _, a := range acquired {
			_ = locker.Release(a.target, a.lockID)
		}
	}

	for _, target := range targets {
		info, err := locker.Acquire(target, operation, ensureDir)
		if err != nil {
			releaseFn()

			return func() {}, err
		}

		if info == nil {
			// Lock skipped (e.g. plan on a not-yet-deployed project).
			continue
		}

		_, _ = fmt.Fprintf(w, "Lock acquired: %s/%s\n", target.Host.Name, target.Project)
		acquired = append(acquired, acquiredLock{target: target, lockID: info.ID})
	}

	return releaseFn, nil
}
