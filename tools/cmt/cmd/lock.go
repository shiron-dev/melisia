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
	w io.Writer,
) (func(), error) {
	var acquired []acquiredLock

	releaseFn := func() {
		for _, a := range acquired {
			_ = locker.Release(a.target, a.lockID)
		}
	}

	for _, target := range targets {
		info, err := locker.Acquire(target, operation)
		if err != nil {
			releaseFn()

			return func() {}, err
		}

		_, _ = fmt.Fprintf(w, "Lock acquired: %s/%s\n", target.Host.Name, target.Project)
		acquired = append(acquired, acquiredLock{target: target, lockID: info.ID})
	}

	return releaseFn, nil
}
