package syncer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"
)

type ApplyDependencies struct {
	ClientFactory remote.ClientFactory
	SSHResolver   config.SSHConfigResolver
	Input         io.Reader
	HookRunner    HookRunner
	ConfigPath    string
}

var (
	ErrExistenceCheckFailed = errors.New(
		"one or more directory existence checks failed (SSH unreachable); fix connectivity and re-run",
	)
	errHookFailed        = errors.New("hook failed")
	errUnknownHookResult = errors.New("unknown hook result")
)

func Apply(ctx context.Context, cfg *config.CmtConfig, plan *SyncPlan, autoApprove bool, w io.Writer) error {
	var dependencies ApplyDependencies

	return ApplyWithDeps(ctx, cfg, plan, autoApprove, false, w, dependencies)
}

func ApplyWithDeps(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	autoApprove bool,
	refreshManifestOnNoop bool,
	writer io.Writer,
	deps ApplyDependencies,
) error {
	style := newOutputStyle(writer)
	clientFactory, input, hookRunner := resolveApplyDependencies(deps)

	shouldContinue, err := runApplyPreflight(
		ctx,
		cfg,
		plan,
		autoApprove,
		refreshManifestOnNoop,
		clientFactory,
		input,
		hookRunner,
		writer,
		style,
		deps,
	)
	if err != nil {
		return err
	}

	if !shouldContinue {
		return nil
	}

	_, _ = fmt.Fprintln(writer)

	err = applyAllHosts(ctx, cfg, plan, clientFactory, writer, style)
	if err != nil {
		return err
	}

	printApplySummary(plan, writer, style)

	if PlanHasExistenceUnknown(plan) {
		return ErrExistenceCheckFailed
	}

	return nil
}

func runApplyPreflight(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	autoApprove bool,
	refreshManifestOnNoop bool,
	clientFactory remote.ClientFactory,
	input io.Reader,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
	deps ApplyDependencies,
) (bool, error) {
	handled, err := handleNoChanges(ctx, plan, refreshManifestOnNoop, clientFactory, writer, style)
	if handled || err != nil {
		return false, err
	}

	cancelled, hookErr := runBeforePlanApplyHook(ctx, cfg, plan, deps, hookRunner, writer, style)
	if hookErr != nil || cancelled {
		return false, hookErr
	}

	plan.Print(writer)

	cancelled, hookErr = runBeforeApplyPromptHook(ctx, cfg, plan, deps, hookRunner, writer, style)
	if hookErr != nil || cancelled {
		return false, hookErr
	}

	if confirmApplyOrCancel(ctx, autoApprove, input, writer, style) {
		return false, nil
	}

	cancelled, hookErr = runBeforeApplyHook(ctx, cfg, plan, deps, hookRunner, writer, style)
	if hookErr != nil || cancelled {
		return false, hookErr
	}

	return true, nil
}

func runBeforePlanApplyHook(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	deps ApplyDependencies,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if cfg.BeforeApplyHooks == nil {
		return false, nil
	}

	payload := buildBeforePlanPayload(plan, deps.ConfigPath, cfg.BasePath)

	return executeApplyHook(
		ctx,
		cfg.BeforeApplyHooks.BeforePlan,
		payload,
		"beforePlan",
		hookRunner,
		writer,
		style,
	)
}

func runBeforeApplyPromptHook(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	deps ApplyDependencies,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if cfg.BeforeApplyHooks == nil {
		return false, nil
	}

	payload := buildBeforeApplyPromptPayload(plan, deps.ConfigPath, cfg.BasePath)

	return executeApplyHook(
		ctx,
		cfg.BeforeApplyHooks.BeforeApplyPrompt,
		payload,
		"beforeApplyPrompt",
		hookRunner,
		writer,
		style,
	)
}

func runBeforeApplyHook(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	deps ApplyDependencies,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if cfg.BeforeApplyHooks == nil {
		return false, nil
	}

	payload := buildBeforeApplyPayload(plan, deps.ConfigPath, cfg.BasePath)

	return executeApplyHook(
		ctx,
		cfg.BeforeApplyHooks.BeforeApply,
		payload,
		"beforeApply",
		hookRunner,
		writer,
		style,
	)
}

func handleNoChanges(
	ctx context.Context,
	plan *SyncPlan,
	refreshManifestOnNoop bool,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	if plan.HasChanges() {
		return false, nil
	}

	_, _ = fmt.Fprintln(writer, style.muted("No changes to apply."))

	if !refreshManifestOnNoop {
		return true, nil
	}

	err := refreshManifestForAllHosts(ctx, plan, clientFactory, writer, style)
	if err != nil {
		return true, err
	}

	_, _ = fmt.Fprintln(writer, style.success("Manifest refreshed."))

	return true, nil
}

func executeApplyHook(
	ctx context.Context,
	hookCmd *config.HookCommand,
	payload any,
	hookName string,
	hookRunner HookRunner,
	writer io.Writer,
	style outputStyle,
) (bool, error) {
	result := runHook(ctx, hookCmd, payload, hookName, hookRunner, writer, style)
	switch result {
	case hookContinue:
		return false, nil
	case hookRejected:
		_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled by hook."))

		return true, nil
	case hookError:
		return false, fmt.Errorf("%w: %s", errHookFailed, hookName)
	}

	return false, errUnknownHookResult
}

func confirmApplyOrCancel(
	ctx context.Context,
	autoApprove bool,
	input io.Reader,
	writer io.Writer,
	style outputStyle,
) bool {
	if autoApprove {
		return false
	}

	if confirmApply(ctx, input, writer, style) {
		return false
	}

	_, _ = fmt.Fprintln(writer, style.warning("Apply cancelled."))

	return true
}

func refreshManifestForAllHosts(
	ctx context.Context,
	plan *SyncPlan,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) error {
	for _, hostPlan := range plan.HostPlans {
		_, _ = fmt.Fprintf(
			writer,
			"%s %s...\n",
			style.key("Refreshing manifest on"),
			style.projectName(hostPlan.Host.Name),
		)

		client, err := clientFactory.NewClient(hostPlan.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hostPlan.Host.Name, err)
		}

		refreshErr := refreshHostManifest(ctx, hostPlan, client, writer, style)
		_ = client.Close()

		if refreshErr != nil {
			return refreshErr
		}
	}

	return nil
}

func refreshHostManifest(
	ctx context.Context,
	hostPlan HostPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	for _, projectPlan := range hostPlan.Projects {
		localFiles, maskHints := collectManifestInputs(projectPlan)

		_, _ = fmt.Fprintf(
			writer,
			"  %s... ",
			style.projectName(projectPlan.ProjectName),
		)

		err := writeProjectManifest(ctx, projectPlan.RemoteDir, localFiles, maskHints, client)
		if err != nil {
			_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

			return err
		}

		_, _ = fmt.Fprintln(writer, style.success("done"))
	}

	return nil
}

func applyHostPlan(
	ctx context.Context,
	cfg *config.CmtConfig,
	hostPlan HostPlan,
	client remote.RemoteClient,
	writer io.Writer,
) error {
	style := newOutputStyle(writer)

	for _, projectPlan := range hostPlan.Projects {
		if !projectPlan.HasChanges() {
			_, _ = fmt.Fprintf(writer, "  %s: %s\n", style.projectName(projectPlan.ProjectName), style.muted("no changes"))

			continue
		}

		_, _ = fmt.Fprintf(writer, "  %s:\n", style.projectName(projectPlan.ProjectName))

		err := applyProjectPlan(ctx, cfg, hostPlan, projectPlan, client, writer, style)
		if err != nil {
			return err
		}
	}

	return nil
}

func resolveApplyDependencies(deps ApplyDependencies) (remote.ClientFactory, io.Reader, HookRunner) {
	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		defaultFactory := new(remote.DefaultClientFactory)
		defaultFactory.Runner = nil
		clientFactory = *defaultFactory
	}

	input := deps.Input
	if input == nil {
		input = os.Stdin
	}

	hookRunner := deps.HookRunner
	if hookRunner == nil {
		hookRunner = defaultHookRunner
	}

	return clientFactory, input, hookRunner
}

func confirmApply(ctx context.Context, input io.Reader, writer io.Writer, style outputStyle) bool {
	_, _ = fmt.Fprint(writer, "\n"+style.key("Apply these changes? (y/N): "))

	answer, err := readLineWithContext(ctx, input)
	if err != nil {
		// Context cancelled (Ctrl+C) or read failure: do not apply.
		return false
	}

	answer = strings.TrimSpace(strings.ToLower(answer))

	return answer == "y" || answer == "yes"
}

// readLineWithContext reads one line from r, returning early if ctx is cancelled.
// A blocking stdin read can't itself be interrupted, so the reader runs in a
// goroutine; on cancellation that goroutine is abandoned, which is safe because
// the process is shutting down (the leak is bounded by exit).
func readLineWithContext(ctx context.Context, r io.Reader) (string, error) {
	type lineResult struct {
		line string
		err  error
	}

	ch := make(chan lineResult, 1)

	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		ch <- lineResult{line: line, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.line, res.err
	}
}

func applyAllHosts(
	ctx context.Context,
	cfg *config.CmtConfig,
	plan *SyncPlan,
	clientFactory remote.ClientFactory,
	writer io.Writer,
	style outputStyle,
) error {
	for _, hostPlan := range plan.HostPlans {
		_, _ = fmt.Fprintf(writer, "%s %s...\n", style.key("Applying to"), style.projectName(hostPlan.Host.Name))

		client, err := clientFactory.NewClient(hostPlan.Host)
		if err != nil {
			return fmt.Errorf("connecting to %s: %w", hostPlan.Host.Name, err)
		}

		applyErr := applyHostPlan(ctx, cfg, hostPlan, client, writer)
		_ = client.Close()

		if applyErr != nil {
			return applyErr
		}
	}

	return nil
}

func printApplySummary(plan *SyncPlan, writer io.Writer, style outputStyle) {
	hostCount, projectCount, addCount, modifyCount, deleteCount, unchangedCount := plan.Stats()
	_ = hostCount
	_ = projectCount
	_ = unchangedCount

	_, _ = fmt.Fprintf(
		writer,
		"\n%s %d file(s) synced (%s added, %s modified, %s deleted)",
		style.success("Apply complete!"),
		addCount+modifyCount+deleteCount,
		style.success(strconv.Itoa(addCount)),
		style.warning(strconv.Itoa(modifyCount)),
		style.danger(strconv.Itoa(deleteCount)),
	)

	composeStart, composeRecreate, composeStop := plan.ComposeStats()
	if composeStart > 0 || composeRecreate > 0 || composeStop > 0 {
		_, _ = fmt.Fprintf(writer, ", compose: %s started, %s recreated, %s stopped",
			style.success(strconv.Itoa(composeStart)),
			style.warning(strconv.Itoa(composeRecreate)),
			style.danger(strconv.Itoa(composeStop)),
		)
	}

	_, _ = fmt.Fprintln(writer)
}

func projectHasChanges(projectPlan ProjectPlan) bool {
	return projectPlan.HasChanges()
}

func applyProjectPlan(
	ctx context.Context,
	_ *config.CmtConfig,
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	err := createMissingDirs(ctx, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	localFiles, maskHints, err := syncProjectFiles(ctx, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = writeProjectManifest(ctx, projectPlan.RemoteDir, localFiles, maskHints, client)
	if err != nil {
		return err
	}

	err = runPostSyncCommand(ctx, hostPlan, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = runComposeAction(ctx, hostPlan, projectPlan, client, writer, style)
	if err != nil {
		return err
	}

	return nil
}

func createMissingDirs(
	ctx context.Context,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	for _, dirPlan := range projectPlan.Dirs {
		if !shouldProcessDir(dirPlan) {
			continue
		}

		err := processDirPlan(ctx, dirPlan, client, writer, style)
		if err != nil {
			return err
		}
	}

	return nil
}

func shouldProcessDir(dirPlan DirPlan) bool {
	applyRecursiveOwnership := dirPlan.Recursive && (dirPlan.Owner != "" || dirPlan.Group != "") && dirPlan.Exists

	return dirPlan.Action != ActionUnchanged || applyRecursiveOwnership
}

func processDirPlan(
	ctx context.Context,
	dirPlan DirPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	actionLabel := dirActionLabel(dirPlan.Exists)
	_, _ = fmt.Fprintf(writer, "    %s %s/... ", style.key(actionLabel), dirPlan.RelativePath)

	err := ensureDirExists(ctx, dirPlan, client, writer, style)
	if err != nil {
		return err
	}

	err = applyDirMetadata(ctx, dirPlan, client, writer, style)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(writer, style.success("done"))

	return nil
}

func dirActionLabel(exists bool) string {
	if exists {
		return "updating dir"
	}

	return "creating dir"
}

func ensureDirExists(
	ctx context.Context,
	dirPlan DirPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	if dirPlan.Exists {
		return nil
	}

	err := client.MkdirAll(ctx, dirPlan.RemotePath)
	if err != nil {
		_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

		return fmt.Errorf("creating directory %s: %w", dirPlan.RemotePath, err)
	}

	return nil
}

func applyDirMetadata(
	ctx context.Context,
	dirPlan DirPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	applyRecursiveOwnership := dirPlan.Recursive && (dirPlan.Owner != "" || dirPlan.Group != "") && dirPlan.Exists

	if dirPlan.NeedsOwnerChange || applyRecursiveOwnership {
		err := applyDirOwnership(ctx, dirPlan, client)
		if err != nil {
			_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

			return err
		}
	}

	if dirPlan.NeedsPermChange {
		err := applyDirPermission(ctx, dirPlan, client)
		if err != nil {
			_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

			return err
		}
	}

	return nil
}

func applyDirPermission(ctx context.Context, dirPlan DirPlan, client remote.RemoteClient) error {
	if dirPlan.Permission == "" {
		return nil
	}

	cmd := buildDirMetadataCommand(
		dirPlan,
		fmt.Sprintf("chmod %s %s", dirPlan.Permission, shellQuote(dirPlan.RemotePath)),
	)

	_, err := client.RunCommand(ctx, "", cmd)
	if err != nil {
		return fmt.Errorf("chmod %s on %s: %w", dirPlan.Permission, dirPlan.RemotePath, err)
	}

	return nil
}

func applyDirOwnership(ctx context.Context, dirPlan DirPlan, client remote.RemoteClient) error {
	if dirPlan.Owner == "" && dirPlan.Group == "" {
		return nil
	}

	ownership := dirPlan.Owner
	if dirPlan.Group != "" {
		ownership += ":" + dirPlan.Group
	}

	chownCmd := "chown"
	if dirPlan.Recursive {
		chownCmd = "chown -R"
	}

	cmd := buildDirMetadataCommand(
		dirPlan,
		fmt.Sprintf("%s %s %s", chownCmd, ownership, shellQuote(dirPlan.RemotePath)),
	)

	_, err := client.RunCommand(ctx, "", cmd)
	if err != nil {
		return fmt.Errorf("chown %s on %s: %w", ownership, dirPlan.RemotePath, err)
	}

	return nil
}

func buildDirMetadataCommand(dirPlan DirPlan, baseCommand string) string {
	if !dirPlan.Become {
		return baseCommand
	}

	if dirPlan.BecomeUser == "" || dirPlan.BecomeUser == "root" {
		return "sudo -n " + baseCommand
	}

	return fmt.Sprintf("sudo -n -u %s %s", shellQuote(dirPlan.BecomeUser), baseCommand)
}

func shellQuote(input string) string {
	return "'" + strings.ReplaceAll(input, "'", "'\\''") + "'"
}

func syncProjectFiles(
	ctx context.Context,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) (map[string]string, map[string][]MaskHint, error) {
	localFiles, maskHints := collectManifestInputs(projectPlan)

	for _, filePlan := range projectPlan.Files {
		switch filePlan.Action {
		case ActionAdd, ActionModify:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("uploading"), filePlan.RelativePath)

			err := client.WriteFile(ctx, filePlan.RemotePath, filePlan.LocalData)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, nil, fmt.Errorf("writing %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))

		case ActionDelete:
			_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("deleting"), filePlan.RelativePath)

			err := client.Remove(ctx, filePlan.RemotePath)
			if err != nil {
				_, _ = fmt.Fprintln(writer, style.danger("FAILED"))

				return nil, nil, fmt.Errorf("deleting %s: %w", filePlan.RemotePath, err)
			}

			_, _ = fmt.Fprintln(writer, style.success("done"))
		case ActionUnchanged:
			// Manifest inputs are collected before this loop.
			continue
		}
	}

	return localFiles, maskHints, nil
}

func collectManifestInputs(projectPlan ProjectPlan) (map[string]string, map[string][]MaskHint) {
	localFiles := make(map[string]string)
	maskHints := make(map[string][]MaskHint)

	for _, filePlan := range projectPlan.Files {
		if filePlan.Action == ActionDelete {
			continue
		}

		localFiles[filePlan.RelativePath] = filePlan.LocalPath

		if len(filePlan.MaskHints) > 0 {
			maskHints[filePlan.RelativePath] = append([]MaskHint(nil), filePlan.MaskHints...)
		}
	}

	return localFiles, maskHints
}

func writeProjectManifest(
	ctx context.Context,
	remoteDir string,
	localFiles map[string]string,
	maskHints map[string][]MaskHint,
	client remote.RemoteClient,
) error {
	manifest := BuildManifestWithMaskHints(localFiles, maskHints)

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling manifest: %w", err)
	}

	manifestPath := path.Join(remoteDir, manifestFile)

	err = client.WriteFile(ctx, manifestPath, manifestData)
	if err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

func runPostSyncCommand(
	ctx context.Context,
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	if projectPlan.PostSyncCommand == "" {
		return nil
	}

	_, _ = fmt.Fprintf(writer, "    %s... ", style.key("running post-sync command"))

	output, err := client.RunCommand(ctx, projectPlan.RemoteDir, projectPlan.PostSyncCommand)
	if err != nil {
		_, _ = fmt.Fprintln(writer, style.danger("FAILED"))
		if output != "" {
			_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("output:"), output)
		}

		return fmt.Errorf("post-sync command on %s/%s: %w", hostPlan.Host.Name, projectPlan.ProjectName, err)
	}

	_, _ = fmt.Fprintln(writer, style.success("done"))

	if output == "" {
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		_, _ = fmt.Fprintf(writer, "      %s\n", line)
	}

	return nil
}

func runComposeAction(
	ctx context.Context,
	hostPlan HostPlan,
	projectPlan ProjectPlan,
	client remote.RemoteClient,
	writer io.Writer,
	style outputStyle,
) error {
	cmd, shouldRun := composeCommand(projectPlan)
	if !shouldRun {
		return nil
	}

	_, _ = fmt.Fprintf(writer, "    %s %s... ", style.key("compose"), cmd)

	output, err := client.RunCommand(ctx, projectPlan.RemoteDir, cmd)
	if err != nil {
		_, _ = fmt.Fprintln(writer, style.danger("FAILED"))
		if output != "" {
			_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("output:"), output)
		}

		return fmt.Errorf("compose %s on %s/%s: %w", cmd, hostPlan.Host.Name, projectPlan.ProjectName, err)
	}

	_, _ = fmt.Fprintln(writer, style.success("done"))

	if output == "" {
		return nil
	}

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		_, _ = fmt.Fprintf(writer, "      %s\n", line)
	}

	return nil
}

func composeCommand(projectPlan ProjectPlan) (string, bool) {
	if projectPlan.Compose == nil {
		return "", false
	}

	switch projectPlan.Compose.ActionType {
	case ComposeNoChange:
		return "", false
	case ComposeStartServices:
		return "docker compose up -d", true
	case ComposeRecreateServices:
		cmd := "docker compose up -d --force-recreate"
		if projectPlan.RemoveOrphans {
			cmd += " --remove-orphans"
		}

		return cmd, true
	case ComposeStopServices:
		cmd := "docker compose down"
		if projectPlan.RemoveOrphans {
			cmd += " --remove-orphans"
		}

		return cmd, true
	default:
		return "", false
	}
}
