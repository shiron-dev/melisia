package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

type HookRunner func(command string, workdir string, stdinData []byte) (exitCode int, combinedOutput string, err error)

type hookResult int

const (
	hookContinue hookResult = iota
	hookRejected
	hookError
)

func defaultHookRunner(command string, workdir string, stdinData []byte) (int, string, error) {
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "eval \"$CMT_HOOK_COMMAND\"")
	if workdir != "" {
		cmd.Dir = filepath.Clean(workdir)
	}

	cmd.Env = append(os.Environ(), "CMT_HOOK_COMMAND="+command)
	cmd.Stdin = bytes.NewReader(stdinData)

	var output bytes.Buffer

	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if err == nil {
		return 0, output.String(), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), output.String(), nil
	}

	return -1, output.String(), fmt.Errorf("executing hook command: %w", err)
}

func runHook(
	hookCmd *config.HookCommand,
	payload any,
	hookName string,
	runner HookRunner,
	writer io.Writer,
	style outputStyle,
) hookResult {
	if hookCmd == nil || hookCmd.Command == "" {
		return hookContinue
	}

	stdinData, err := json.Marshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(writer, "%s %s: %v\n", style.danger("Hook error"), hookName, err)

		return hookError
	}

	_, _ = fmt.Fprintf(writer, "\n%s %s...\n", style.key("Running hook"), hookName)

	exitCode, output, err := runner(hookCmd.Command, hookWorkdir(payload), stdinData)
	if err != nil {
		_, _ = fmt.Fprintf(writer, "%s %s: %v\n", style.danger("Hook error"), hookName, err)
		printHookOutput(output, writer)

		return hookError
	}

	printHookOutput(output, writer)

	switch exitCode {
	case 0:
		_, _ = fmt.Fprintf(writer, "%s %s passed.\n", style.success("Hook"), hookName)

		return hookContinue
	case 1:
		_, _ = fmt.Fprintf(writer, "%s %s rejected apply.\n", style.warning("Hook"), hookName)

		return hookRejected
	default:
		_, _ = fmt.Fprintf(writer, "%s %s exited with code %d.\n", style.danger("Hook error"), hookName, exitCode)

		return hookError
	}
}

func printHookOutput(output string, writer io.Writer) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return
	}

	for line := range strings.SplitSeq(trimmed, "\n") {
		_, _ = fmt.Fprintf(writer, "  %s\n", line)
	}
}

func buildHookPayloadData(
	configPath string,
	basePath string,
) config.HookConfigPaths {
	return config.HookConfigPaths{
		ConfigPath: configPath,
		BasePath:   basePath,
	}
}

func buildBeforePlanPayload(plan *SyncPlan, configPath, basePath string) config.BeforePlanHookPayload {
	hosts := collectHostNames(plan)
	pwd, _ := os.Getwd()

	return config.BeforePlanHookPayload{
		Hosts:      hosts,
		WorkingDir: pwd,
		Paths:      buildHookPayloadData(configPath, basePath),
	}
}

func buildBeforeApplyPromptPayload(plan *SyncPlan, configPath, basePath string) config.BeforeApplyPromptHookPayload {
	hosts := collectHostNames(plan)
	pwd, _ := os.Getwd()

	return config.BeforeApplyPromptHookPayload{
		Hosts:      hosts,
		WorkingDir: pwd,
		Paths:      buildHookPayloadData(configPath, basePath),
	}
}

func buildBeforeApplyPayload(plan *SyncPlan, configPath, basePath string) config.BeforeApplyHookPayload {
	hosts := collectHostNames(plan)
	pwd, _ := os.Getwd()

	return config.BeforeApplyHookPayload{
		Hosts:      hosts,
		WorkingDir: pwd,
		Paths:      buildHookPayloadData(configPath, basePath),
	}
}

func collectHostNames(plan *SyncPlan) []string {
	names := make([]string, 0, len(plan.HostPlans))

	for _, hp := range plan.HostPlans {
		names = append(names, hp.Host.Name)
	}

	return names
}

func hookWorkdir(payload any) string {
	switch hookPayload := payload.(type) {
	case config.BeforePlanHookPayload:
		return hookPayload.Paths.BasePath
	case config.BeforeApplyPromptHookPayload:
		return hookPayload.Paths.BasePath
	case config.BeforeApplyHookPayload:
		return hookPayload.Paths.BasePath
	default:
		return ""
	}
}
