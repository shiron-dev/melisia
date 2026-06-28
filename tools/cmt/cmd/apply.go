package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
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
	applyCommand.RunE = func(cmd *cobra.Command, _ []string) error {
		return runApplyCmd(
			cmd.Context(),
			*configPath,
			hostFilter,
			projectFilter,
			autoApprove,
			refreshManifestOnNoop,
			expectedPlanSHA256,
			applyDependencies,
		)
	}

	bindApplyFlags(applyCommand, &hostFilter, &projectFilter, &autoApprove, &refreshManifestOnNoop, &expectedPlanSHA256)

	return applyCommand
}

func runApplyCmd(
	ctx context.Context,
	configPath string,
	hostFilter, projectFilter []string,
	autoApprove bool,
	refreshManifestOnNoop bool,
	expectedPlanSHA256 string,
	applyDependencies syncer.ApplyDependencies,
) (retErr error) {
	cfg, err := config.LoadCmtConfig(configPath)
	if err != nil {
		return err
	}

	release, err := acquireApplyLocks(ctx, cfg, hostFilter, projectFilter, applyDependencies)
	if err != nil {
		return err
	}

	defer func() {
		releaseErr := release()
		if releaseErr != nil && retErr == nil {
			retErr = releaseErr
		}
	}()

	planDependencies := new(syncer.PlanDependencies)
	planDependencies.ClientFactory = applyDependencies.ClientFactory
	planDependencies.SSHResolver = applyDependencies.SSHResolver
	planDependencies.ProgressWriter = os.Stdout

	plan, err := syncer.BuildPlanWithDeps(ctx, cfg, hostFilter, projectFilter, *planDependencies)
	if err != nil {
		return err
	}

	err = verifyExpectedPlanSHA256(plan, expectedPlanSHA256)
	if err != nil {
		return err
	}

	applyDependencies.ConfigPath = configPath

	return syncer.ApplyWithDeps(
		ctx,
		cfg,
		plan,
		autoApprove,
		refreshManifestOnNoop,
		os.Stdout,
		applyDependencies,
	)
}

// acquireApplyLocks resolves the apply targets and takes their remote locks.
func acquireApplyLocks(
	ctx context.Context,
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	applyDependencies syncer.ApplyDependencies,
) (func() error, error) {
	lockDeps := syncer.PlanDependencies{
		ClientFactory:  applyDependencies.ClientFactory,
		SSHResolver:    applyDependencies.SSHResolver,
		LocalRunner:    nil,
		ProgressWriter: nil,
	}

	targets, err := syncer.ResolveLockTargets(ctx, cfg, hostFilter, projectFilter, lockDeps)
	if err != nil {
		return nil, err
	}

	return acquireRemoteLocks(ctx, remoteLocker(applyDependencies.ClientFactory), targets, os.Stdout)
}

// remoteLocker builds a RemoteLocker, defaulting to a real SSH client factory.
func remoteLocker(factory remote.ClientFactory) *lock.RemoteLocker {
	if factory == nil {
		factory = remote.DefaultClientFactory{Runner: nil}
	}

	return lock.NewRemote(factory)
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
	bindProjectFilterFlags(applyCommand, projectFilter)
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
