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
	errUnreadableLockNeedsForce  = errors.New("lock present but unreadable; re-run with --force to remove")
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
	// Lenient resolution: a host that never runs the filtered project is skipped
	// rather than aborting the whole batch.
	targets, err := syncer.ResolveLockTargetsLenient(cfg, opts.hostFilter, opts.projectFilter, deps)
	if err != nil {
		return err
	}

	return forceUnlockMany(locker, targets, opts.force)
}

type lockedTarget struct {
	target lock.Target
	info   *lock.Info
}

// lockScan splits the candidate targets into locks that were read successfully
// and locks that are present but unreadable (corrupt JSON, permission). Targets
// with no lock at all are dropped.
type lockScan struct {
	locked     []lockedTarget
	unreadable []lock.Target
}

func (s lockScan) empty() bool {
	return len(s.locked) == 0 && len(s.unreadable) == 0
}

func (s lockScan) count() int {
	return len(s.locked) + len(s.unreadable)
}

func scanLockedTargets(locker *lock.RemoteLocker, candidates []lock.Target) lockScan {
	var scan lockScan

	for _, target := range candidates {
		info, readErr := locker.Read(target)

		switch {
		case readErr == nil:
			scan.locked = append(scan.locked, lockedTarget{target: target, info: info})
		case errors.Is(readErr, lock.ErrLockNotFound):
			// No lock here; nothing to release.
		default:
			scan.unreadable = append(scan.unreadable, target)
		}
	}

	return scan
}

// forceUnlockMany scans the candidate targets, force-unlocks only the ones that
// are currently locked, and asks for a single confirmation covering the batch.
func forceUnlockMany(locker *lock.RemoteLocker, candidates []lock.Target, force bool) error {
	scan := scanLockedTargets(locker, candidates)

	if scan.empty() {
		_, _ = fmt.Fprintln(os.Stdout, "No locks found to release.")

		return nil
	}

	for _, lt := range scan.locked {
		printLockInfo(lt.target, lt.info)
	}

	for _, target := range scan.unreadable {
		_, _ = fmt.Fprintf(os.Stdout, "Lock present but unreadable for %s/%s.\n", target.Host.Name, target.Project)
	}

	if !force && !confirmForceUnlock(fmt.Sprintf("%d lock(s)", scan.count())) {
		_, _ = fmt.Fprintln(os.Stdout, "Force-unlock cancelled.")

		return nil
	}

	return releaseScannedLocks(locker, scan, force)
}

// releaseScannedLocks removes every readable lock and, when --force is set, every
// unreadable one too (mirroring the single-target force path). Without --force an
// unreadable lock can't be safely removed, so it is reported as an error.
func releaseScannedLocks(locker *lock.RemoteLocker, scan lockScan, force bool) error {
	var firstErr error

	for _, lt := range scan.locked {
		err := locker.ForceUnlockWithID(lt.target, lt.info.ID)
		firstErr = reportRelease(lt.target, err, firstErr)
	}

	for _, target := range scan.unreadable {
		if !force {
			_, _ = fmt.Fprintf(os.Stderr,
				"Warning: not removing unreadable lock %s/%s without --force.\n",
				target.Host.Name, target.Project)

			if firstErr == nil {
				firstErr = errUnreadableLockNeedsForce
			}

			continue
		}

		err := locker.ForceUnlock(target)
		firstErr = reportRelease(target, err, firstErr)
	}

	return firstErr
}

// reportRelease prints the outcome of a single release and folds any failure into
// the running first error.
func reportRelease(target lock.Target, err, firstErr error) error {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to release lock %s/%s: %v\n",
			target.Host.Name, target.Project, err)

		if firstErr == nil {
			return err
		}

		return firstErr
	}

	_, _ = fmt.Fprintf(os.Stdout, "Lock released for %s/%s.\n", target.Host.Name, target.Project)

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
