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

var (
	errLockTargetNotResolved     = errors.New("could not resolve lock target")
	errForceUnlockNeedsTwoArgs   = errors.New("force-unlock requires <host> <project> (or use --all)")
	errForceUnlockAllNoArgs      = errors.New("--all takes no positional args; narrow the scope with --host/--project")
	errForceUnlockFiltersNeedAll = errors.New("--host/--project may only be used together with --all")
)

const forceUnlockArgCount = 2

// forceUnlockOptions captures the flags that select which locks are released.
type forceUnlockOptions struct {
	force         bool
	all           bool
	hostFilter    []string
	projectFilter []string
}

func newForceUnlockCmd(configPath *string) *cobra.Command {
	var opts forceUnlockOptions

	cmd := new(cobra.Command)
	cmd.Use = "force-unlock [<host> <project>]"
	cmd.Short = "Release a stuck lock for a project on a host"
	cmd.Long = `Remove the remote lock file for a project that was left locked by a crashed or interrupted operation.

The lock lives on the remote host at <remotePath>/<project>/.cmt.lock.

Pass --all to release every lock that is currently held, optionally narrowed with
the repeatable --host / --project filters:

  cmt force-unlock --all                       # every locked project on every host
  cmt force-unlock --all --host arm-srv        # every locked project on arm-srv
  cmt force-unlock --all --project grafana     # grafana on every host that has it locked

With --all only projects that are currently locked are touched and a single
confirmation covers the whole batch.

Use --force to skip the confirmation prompt.`
	cmd.Args = cobra.MaximumNArgs(forceUnlockArgCount)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		return runForceUnlock(*configPath, args, opts)
	}

	cmd.Flags().BoolVar(&opts.force, "force", false, "skip confirmation prompt")
	cmd.Flags().BoolVar(&opts.all, "all", false, "release every currently held lock (narrow with --host/--project)")
	cmd.Flags().StringSliceVar(&opts.hostFilter, "host", nil, "with --all: filter by host name (repeatable)")
	cmd.Flags().StringSliceVar(&opts.projectFilter, "project", nil, "with --all: filter by project name (repeatable)")

	return cmd
}

func runForceUnlock(configPath string, args []string, opts forceUnlockOptions) error {
	err := validateForceUnlockArgs(args, opts)
	if err != nil {
		return err
	}

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

	if opts.all {
		return runForceUnlockAll(remoteLocker(nil), cfg, opts, deps)
	}

	// validateForceUnlockArgs already guaranteed exactly two args here; the guard
	// keeps the indexing provably safe for static analysis.
	if len(args) < forceUnlockArgCount {
		return errForceUnlockNeedsTwoArgs
	}

	target, err := resolveSingleLockTarget(cfg, args[0], args[1], deps)
	if err != nil {
		return err
	}

	return runForceUnlockWithLocker(remoteLocker(nil), target, opts.force)
}

// validateForceUnlockArgs rejects flag/argument combinations that don't make
// sense: --all takes no positionals, the filters require --all, and the
// single-target form needs exactly <host> <project>.
func validateForceUnlockArgs(args []string, opts forceUnlockOptions) error {
	if opts.all {
		if len(args) > 0 {
			return errForceUnlockAllNoArgs
		}

		return nil
	}

	if len(opts.hostFilter) > 0 || len(opts.projectFilter) > 0 {
		return errForceUnlockFiltersNeedAll
	}

	if len(args) != forceUnlockArgCount {
		return errForceUnlockNeedsTwoArgs
	}

	return nil
}

func runForceUnlockAll(
	locker *lock.RemoteLocker,
	cfg *config.CmtConfig,
	opts forceUnlockOptions,
	deps syncer.PlanDependencies,
) error {
	targets, err := syncer.ResolveLockTargets(cfg, opts.hostFilter, opts.projectFilter, deps)
	if err != nil {
		return err
	}

	return forceUnlockMany(locker, targets, opts.force)
}

type lockedTarget struct {
	target lock.Target
	info   *lock.Info
}

// scanLockedTargets reads each candidate and returns only those currently
// locked. A genuine read failure (SSH/permission) shouldn't abort the whole
// sweep, so it is warned-and-skipped rather than propagated.
func scanLockedTargets(locker *lock.RemoteLocker, candidates []lock.Target) []lockedTarget {
	var locked []lockedTarget

	for _, target := range candidates {
		info, readErr := locker.Read(target)
		if readErr != nil {
			if !errors.Is(readErr, lock.ErrLockNotFound) {
				_, _ = fmt.Fprintf(os.Stderr, "Warning: skipping %s/%s: %v\n",
					target.Host.Name, target.Project, readErr)
			}

			continue
		}

		locked = append(locked, lockedTarget{target: target, info: info})
	}

	return locked
}

// forceUnlockMany scans the candidate targets, force-unlocks only the ones that
// are currently locked, and asks for a single confirmation covering the batch.
func forceUnlockMany(locker *lock.RemoteLocker, candidates []lock.Target, force bool) error {
	locked := scanLockedTargets(locker, candidates)

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
