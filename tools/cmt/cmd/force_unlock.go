package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/spf13/cobra"
)

func newForceUnlockCmd() *cobra.Command {
	var force bool

	cmd := new(cobra.Command)
	cmd.Use = "force-unlock <host>"
	cmd.Short = "Release a stuck lock for a host"
	cmd.Long = `Remove the lock file for a host that was left locked by a crashed or interrupted operation.

Use --force to skip the confirmation prompt.`
	cmd.Args = cobra.ExactArgs(1)
	cmd.RunE = func(_ *cobra.Command, args []string) error {
		return runForceUnlock(args[0], force)
	}

	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")

	return cmd
}

func runForceUnlock(hostName string, force bool) error {
	locker := lock.New()

	info, readErr := locker.Read(hostName)
	if readErr != nil {
		if errors.Is(readErr, lock.ErrLockNotFound) {
			return readErr
		}

		if !force {
			return fmt.Errorf("reading lock for %q: %w", hostName, readErr)
		}

		_, _ = fmt.Fprintf(os.Stderr, "Warning: could not read lock for %q (%v); removing anyway.\n", hostName, readErr)
	}

	if info != nil {
		_, _ = fmt.Fprintf(os.Stdout, "Lock info for host %q:\n", hostName)
		_, _ = fmt.Fprintf(os.Stdout, "  ID:        %s\n", info.ID)
		_, _ = fmt.Fprintf(os.Stdout, "  Operation: %s\n", info.Operation)
		_, _ = fmt.Fprintf(os.Stdout, "  Who:       %s\n", info.Who)
		_, _ = fmt.Fprintf(os.Stdout, "  Created:   %s\n", info.Created.Format(time.RFC3339))
		_, _ = fmt.Fprintln(os.Stdout)
	}

	if !force && !confirmForceUnlock(hostName) {
		_, _ = fmt.Fprintln(os.Stdout, "Force-unlock cancelled.")

		return nil
	}

	err := locker.ForceUnlock(hostName)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(os.Stdout, "Lock released for host %q.\n", hostName)

	return nil
}

func confirmForceUnlock(hostName string) bool {
	_, _ = fmt.Fprintf(os.Stdout, "Do you really want to force-unlock %q? (y/N): ", hostName)

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	return answer == "y" || answer == "yes"
}
