package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"
	"github.com/spf13/cobra"
)

var errLockTargetNotResolved = errors.New("could not resolve lock target")

const (
	forceUnlockArgCount = 2

	// lockWildcard matches every host or project when passed in place of a
	// concrete name, so a single command can release many stuck locks at once.
	// "*" is used (not "all") because it can't collide with a real host or
	// project name; quote it in the shell so it isn't glob-expanded.
	lockWildcard = "*"
)

func newForceUnlockCmd(configPath *string) *cobra.Command {
	var force bool

	cmd := new(cobra.Command)
	cmd.Use = "force-unlock <host> <project>"
	cmd.Short = "Release a stuck lock for a project on a host"
	cmd.Long = `Remove the remote lock file for a project that was left locked by a crashed or interrupted operation.

The lock lives on the remote host at <remotePath>/<project>/.cmt.lock.

Pass "*" in place of <host> and/or <project> to release every matching lock
(quote it so the shell doesn't expand it):

  cmt force-unlock '*' '*'          # every locked project on every host
  cmt force-unlock arm-srv '*'      # every locked project on arm-srv
  cmt force-unlock '*' grafana      # grafana on every host that has it locked

When a wildcard is used, only projects that are currently locked are touched and
a single confirmation covers the whole batch.

Use --force to skip the confirmation prompt.`
	cmd.Args = cobra.ExactArgs(forceUnlockArgCount)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		return runForceUnlock(*configPath, args[0], args[1], force)
	}

	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")

	return cmd
}

func runForceUnlock(configPath, hostName, project string, force bool) error {
	cfg, err := config.LoadCmtConfig(configPath)
	if err != nil {
		return err
	}

	deps := syncer.PlanDependencies{
		ClientFactory:  nil,
		SSHResolver:    nil,
		LocalRunner:    nil,
		ProgressWriter: nil,
	}

	if hostName == lockWildcard || project == lockWildcard {
		return runForceUnlockWildcard(cfg, hostName, project, force, deps)
	}

	target, err := resolveSingleLockTarget(cfg, hostName, project, deps)
	if err != nil {
		return err
	}

	return runForceUnlockWithLocker(remoteLocker(nil), target, force)
}

// lockFilter turns a positional argument into a ResolveLockTargets filter: the
// wildcard becomes an empty filter (matches all), anything else a single name.
func lockFilter(value string) []string {
	if value == lockWildcard {
		return nil
	}

	return []string{value}
}

func runForceUnlockWildcard(
	cfg *config.CmtConfig,
	hostName, project string,
	force bool,
	deps syncer.PlanDependencies,
) error {
	targets, err := syncer.ResolveLockTargets(cfg, lockFilter(hostName), lockFilter(project), deps)
	if err != nil {
		return err
	}

	return forceUnlockMany(remoteLocker(nil), targets, force)
}

// forceUnlockMany scans the candidate targets, force-unlocks only the ones that
// are currently locked, and asks for a single confirmation covering the batch.
func forceUnlockMany(locker *lock.RemoteLocker, candidates []lock.Target, force bool) error {
	type lockedTarget struct {
		target lock.Target
		info   *lock.Info
	}

	var locked []lockedTarget

	for _, target := range candidates {
		info, readErr := locker.Read(target)
		if readErr != nil {
			if errors.Is(readErr, lock.ErrLockNotFound) {
				continue
			}

			// A genuine read failure (SSH/permission) shouldn't abort the whole
			// sweep: warn and keep going so other hosts still get unlocked.
			_, _ = fmt.Fprintf(os.Stderr, "Warning: skipping %s/%s: %v\n",
				target.Host.Name, target.Project, readErr)

			continue
		}

		locked = append(locked, lockedTarget{target: target, info: info})
	}

	if len(locked) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No locks found to release.")

		return nil
	}

	for _, lt := range locked {
		printLockInfo(lt.target, lt.info)
	}

	if !force && !confirmForceUnlock(fmt.Sprintf("%d lock(s)", len(locked))) {
		_, _ = fmt.Fprintln(os.Stdout, "Force-unlock cancelled.")

		return nil
	}

	var firstErr error

	for _, lt := range locked {
		err := locker.ForceUnlockWithID(lt.target, lt.info.ID)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to release lock %s/%s: %v\n",
				lt.target.Host.Name, lt.target.Project, err)

			if firstErr == nil {
				firstErr = err
			}

			continue
		}

		_, _ = fmt.Fprintf(os.Stdout, "Lock released for %s/%s.\n", lt.target.Host.Name, lt.target.Project)
	}

	return firstErr
}

func resolveSingleLockTarget(
	cfg *config.CmtConfig,
	hostName, project string,
	deps syncer.PlanDependencies,
) (lock.Target, error) {
	targets, err := syncer.ResolveLockTargets(cfg, []string{hostName}, []string{project}, deps)
	if err != nil {
		return lock.Target{}, err
	}

	for _, t := range targets {
		if t.Host.Name == hostName && t.Project == project {
			return t, nil
		}
	}

	return lock.Target{}, fmt.Errorf("%w: host %q, project %q", errLockTargetNotResolved, hostName, project)
}

func runForceUnlockWithLocker(locker *lock.RemoteLocker, target lock.Target, force bool) error {
	info, readErr := locker.Read(target)
	if readErr != nil {
		if errors.Is(readErr, lock.ErrLockNotFound) {
			return readErr
		}

		if !force {
			return fmt.Errorf("reading lock for %s/%s: %w", target.Host.Name, target.Project, readErr)
		}

		_, _ = fmt.Fprintf(os.Stderr,
			"Warning: could not read lock for %s/%s (%v); removing anyway.\n",
			target.Host.Name, target.Project, readErr)
	}

	if info != nil {
		printLockInfo(target, info)
	}

	if !force && !confirmForceUnlock(target.Host.Name+"/"+target.Project) {
		_, _ = fmt.Fprintln(os.Stdout, "Force-unlock cancelled.")

		return nil
	}

	var err error
	if info != nil {
		err = locker.ForceUnlockWithID(target, info.ID)
	} else {
		err = locker.ForceUnlock(target)
	}

	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Lock released for %s/%s.\n", target.Host.Name, target.Project)

	return nil
}

func printLockInfo(target lock.Target, info *lock.Info) {
	_, _ = fmt.Fprintf(os.Stdout, "Lock info for %s/%s:\n", target.Host.Name, target.Project)
	_, _ = fmt.Fprintf(os.Stdout, "  ID:        %s\n", info.ID)
	_, _ = fmt.Fprintf(os.Stdout, "  Operation: %s\n", info.Operation)
	_, _ = fmt.Fprintf(os.Stdout, "  Who:       %s\n", info.Who)
	_, _ = fmt.Fprintf(os.Stdout, "  Created:   %s\n", info.Created.Format(time.RFC3339))
	_, _ = fmt.Fprintln(os.Stdout)
}

func confirmForceUnlock(label string) bool {
	_, _ = fmt.Fprintf(os.Stdout, "Do you really want to force-unlock %q? (y/N): ", label)

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	return answer == "y" || answer == "yes"
}
