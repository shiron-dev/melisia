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

const forceUnlockArgCount = 2

func newForceUnlockCmd(configPath *string) *cobra.Command {
	var force bool

	cmd := new(cobra.Command)
	cmd.Use = "force-unlock <host> <project>"
	cmd.Short = "Release a stuck lock for a project on a host"
	cmd.Long = `Remove the remote lock file for a project that was left locked by a crashed or interrupted operation.

The lock lives on the remote host at <remotePath>/<project>/.cmt.lock.

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

	target, err := resolveSingleLockTarget(cfg, hostName, project, deps)
	if err != nil {
		return err
	}

	return runForceUnlockWithLocker(remoteLocker(nil), target, force)
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
