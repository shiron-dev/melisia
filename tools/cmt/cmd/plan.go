package cmd

import (
	"fmt"
	"os"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"

	"github.com/spf13/cobra"
)

const (
	exitCodeNoChanges  = 0
	exitCodeHasChanges = 2
	planDigestFileMode = 0o600
)

func newPlanCmd(configPath *string) *cobra.Command {
	var hostFilter []string

	var projectFilter []string

	var exitCode bool

	var digestFile string

	dependencies := syncer.PlanDependencies{
		ClientFactory:  nil,
		SSHResolver:    nil,
		LocalRunner:    nil,
		ProgressWriter: os.Stdout,
	}

	planCommand := new(cobra.Command)
	planCommand.Use = "plan"
	planCommand.Short = "Show what would be synced without making changes"
	planCommand.RunE = func(_ *cobra.Command, _ []string) error {
		return runPlanCmd(*configPath, hostFilter, projectFilter, exitCode, digestFile, dependencies)
	}

	bindPlanFlags(planCommand, &hostFilter, &projectFilter, &exitCode, &digestFile)

	return planCommand
}

// runPlanCmd builds and prints the plan. plan is read-only, so it does not
// acquire remote locks — a plan must never block (or be blocked by) other
// plan/apply operations.
func runPlanCmd(
	configPath string,
	hostFilter, projectFilter []string,
	exitCode bool,
	digestFile string,
	dependencies syncer.PlanDependencies,
) error {
	cfg, err := config.LoadCmtConfig(configPath)
	if err != nil {
		return err
	}

	plan, err := syncer.BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
	if err != nil {
		return err
	}

	plan.Print(os.Stdout)

	err = writePlanDigestFile(digestFile, plan)
	if err != nil {
		return err
	}

	if syncer.PlanHasExistenceUnknown(plan) {
		return syncer.ErrExistenceCheckFailed
	}

	if exitCode {
		exitWithPlanCode(plan)
	}

	return nil
}

// planExit terminates the process with the plan's exit code. It is a variable
// so tests can observe the chosen code without exiting the test binary.
//
//nolint:gochecknoglobals // overridable seam for testing os.Exit behaviour
var planExit = os.Exit

func exitWithPlanCode(plan *syncer.SyncPlan) {
	if plan.HasChanges() {
		planExit(exitCodeHasChanges)

		return
	}

	planExit(exitCodeNoChanges)
}

func bindPlanFlags(
	planCommand *cobra.Command,
	hostFilter *[]string,
	projectFilter *[]string,
	exitCode *bool,
	digestFile *string,
) {
	planCommand.Flags().StringSliceVar(hostFilter, "host", nil, "filter by host name (repeatable)")
	bindProjectFilterFlags(planCommand, projectFilter)
	planCommand.Flags().BoolVar(exitCode, "exit-code", false,
		"exit with 0 when no changes, 1 on error, 2 when changes exist")
	planCommand.Flags().BoolVar(exitCode, "exit-status", false,
		"alias of --exit-code: exit with 0 when no changes, 1 on error, 2 when changes exist")
	planCommand.Flags().StringVar(digestFile, "digest-file", "",
		"write the SHA-256 digest of the normalized plan to this file")
}

func writePlanDigestFile(digestFile string, plan *syncer.SyncPlan) error {
	if digestFile == "" {
		return nil
	}

	err := os.WriteFile(digestFile, []byte(syncer.PlanDigestSHA256(plan)+"\n"), planDigestFileMode)
	if err != nil {
		return fmt.Errorf("write plan digest: %w", err)
	}

	return nil
}
