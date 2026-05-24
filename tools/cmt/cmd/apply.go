package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"

	"github.com/spf13/cobra"
)

var errPlanDigestMismatch = errors.New("cmt plan changed before apply")

func newApplyCmd(configPath *string) *cobra.Command {
	var hostFilter []string

	var projectFilter []string

	var (
		autoApprove           bool
		refreshManifestOnNoop bool
		expectedPlanSHA256    string
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

		err = verifyExpectedPlanSHA256(plan, expectedPlanSHA256)
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

	bindApplyFlags(applyCommand, &hostFilter, &projectFilter, &autoApprove, &refreshManifestOnNoop, &expectedPlanSHA256)

	return applyCommand
}

func bindApplyFlags(
	applyCommand *cobra.Command,
	hostFilter *[]string,
	projectFilter *[]string,
	autoApprove *bool,
	refreshManifestOnNoop *bool,
	expectedPlanSHA256 *string,
) {
	applyCommand.Flags().StringSliceVar(hostFilter, "host", nil, "filter by host name (repeatable)")
	applyCommand.Flags().StringSliceVar(projectFilter, "project", nil, "filter by project name (repeatable)")
	applyCommand.Flags().BoolVar(autoApprove, "auto-approve", false, "skip confirmation prompt")
	applyCommand.Flags().BoolVar(
		refreshManifestOnNoop,
		"refresh-manifest-on-noop",
		false,
		"refresh .cmt-manifest.json even when no file changes are detected",
	)
	applyCommand.Flags().StringVar(
		expectedPlanSHA256,
		"expected-plan-sha256",
		"",
		"only apply when the internally generated plan matches this SHA-256 digest",
	)
}

func verifyExpectedPlanSHA256(plan *syncer.SyncPlan, expectedPlanSHA256 string) error {
	if expectedPlanSHA256 == "" {
		return nil
	}

	actualPlanSHA256 := syncer.PlanDigestSHA256(plan)
	if strings.EqualFold(actualPlanSHA256, expectedPlanSHA256) {
		return nil
	}

	return fmt.Errorf(
		"%w: expected SHA-256 %s, got %s",
		errPlanDigestMismatch,
		expectedPlanSHA256,
		actualPlanSHA256,
	)
}
