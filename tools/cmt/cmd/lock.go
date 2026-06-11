package cmd

import (
	"fmt"
	"io"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

type acquiredLock struct {
	hostName string
	lockID   string
}

func acquireHostLocks(locker *lock.Locker, hosts []config.HostEntry, operation string, w io.Writer) (func(), error) {
	var acquired []acquiredLock

	releaseFn := func() {
		for _, l := range acquired {
			_ = locker.Release(l.hostName, l.lockID)
		}
	}

	for _, host := range hosts {
		info, err := locker.Acquire(host.Name, operation)
		if err != nil {
			releaseFn()

			return func() {}, err
		}

		_, _ = fmt.Fprintf(w, "Lock acquired: %s\n", host.Name)
		acquired = append(acquired, acquiredLock{hostName: host.Name, lockID: info.ID})
	}

	return releaseFn, nil
}
