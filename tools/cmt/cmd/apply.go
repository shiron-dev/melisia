package cmd

import (
	"os"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"

	"github.com/spf13/cobra"
)

func newApplyCmd(configPath *string) *cobra.Command {
	var hostFilter []string

	var projectFilter []string

	var (
		autoApprove           bool
		refreshManifestOnNoop bool
	)

	var applyDependencies syncer.ApplyDependencies

	applyCommand := new(cobra.Command)
	applyCommand.Use = "apply"
	applyCommand.Short = "Sync files to remote hosts (with confirmation unless --auto-approve)"
	applyCommand.RunE = func(_ *cobra.Command, _ []string) error {
		cfg, err := config.LoadCmtConfig(*configPath)
		if err != nil {
			return err
		}

		planDependencies := new(syncer.PlanDependencies)
		planDependencies.ClientFactory = applyDependencies.ClientFactory
		planDependencies.SSHResolver = nil
		planDependencies.ProgressWriter = os.Stdout

		plan, err := syncer.BuildPlanWithDeps(cfg, hostFilter, projectFilter, *planDependencies)
		if err != nil {
			return err
		}

		applyDependencies.ConfigPath = *configPath

		return syncer.ApplyWithDeps(
			cfg,
			plan,
			autoApprove,
			refreshManifestOnNoop,
			os.Stdout,
			applyDependencies,
		)
	}

	applyCommand.Flags().StringSliceVar(&hostFilter, "host", nil, "filter by host name (repeatable)")
	applyCommand.Flags().StringSliceVar(&projectFilter, "project", nil, "filter by project name (repeatable)")
	applyCommand.Flags().BoolVar(&autoApprove, "auto-approve", false, "skip confirmation prompt")
	applyCommand.Flags().BoolVar(
		&refreshManifestOnNoop,
		"refresh-manifest-on-noop",
		false,
		"refresh .cmt-manifest.json even when no file changes are detected",
	)

	return applyCommand
}
