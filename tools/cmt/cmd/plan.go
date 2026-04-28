package cmd

import (
	"os"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"

	"github.com/spf13/cobra"
)

const (
	exitCodeNoChanges  = 0
	exitCodeHasChanges = 2
)

func newPlanCmd(configPath *string) *cobra.Command {
	var hostFilter []string

	var projectFilter []string

	var exitCode bool

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
		cfg, err := config.LoadCmtConfig(*configPath)
		if err != nil {
			return err
		}

		plan, err := syncer.BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
		if err != nil {
			return err
		}

		plan.Print(os.Stdout)

		if syncer.PlanHasExistenceUnknown(plan) {
			return syncer.ErrExistenceCheckFailed
		}

		if exitCode {
			if plan.HasChanges() {
				os.Exit(exitCodeHasChanges)
			}

			os.Exit(exitCodeNoChanges)
		}

		return nil
	}

	planCommand.Flags().StringSliceVar(&hostFilter, "host", nil, "filter by host name (repeatable)")
	planCommand.Flags().StringSliceVar(&projectFilter, "project", nil, "filter by project name (repeatable)")
	planCommand.Flags().BoolVar(&exitCode, "exit-code", false,
		"exit with 0 when no changes, 1 on error, 2 when changes exist")
	planCommand.Flags().BoolVar(&exitCode, "exit-status", false,
		"alias of --exit-code: exit with 0 when no changes, 1 on error, 2 when changes exist")

	return planCommand
}
