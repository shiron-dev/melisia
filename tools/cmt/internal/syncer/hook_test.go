package syncer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

func TestRunHook_NilCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)

	result := runHook(nil, nil, "test", defaultHookRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v", result)
	}
}

func TestRunHook_EmptyCommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: ""}

	result := runHook(cmd, nil, "test", defaultHookRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v", result)
	}
}

func TestRunHook_ExitZero(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "true"}
	payload := config.BeforePlanHookPayload{Hosts: []string{"server1"}, WorkingDir: "/tmp"}

	result := runHook(cmd, payload, "beforePlan", defaultHookRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v; output: %s", result, out.String())
	}
}

func TestRunHook_ExitOne(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "exit 1"}

	result := runHook(cmd, "payload", "beforePlan", defaultHookRunner, &out, style)
	if result != hookRejected {
		t.Fatalf("expected hookRejected, got %v; output: %s", result, out.String())
	}
}

func TestRunHook_ExitTwo(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "exit 2"}

	result := runHook(cmd, "payload", "test", defaultHookRunner, &out, style)
	if result != hookError {
		t.Fatalf("expected hookError, got %v; output: %s", result, out.String())
	}
}

func TestRunHook_ReceivesStdinJSON(t *testing.T) {
	t.Parallel()

	var captured []byte

	mockRunner := func(command string, workdir string, stdinData []byte) (int, string, error) {
		_ = command
		_ = workdir
		captured = stdinData

		return 0, "", nil
	}

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "cat"}

	payload := config.BeforePlanHookPayload{
		Hosts:      []string{"server1", "server2"},
		WorkingDir: "/work",
		Paths: config.HookConfigPaths{
			ConfigPath: "config.yml",
			BasePath:   "/work/compose",
		},
	}

	result := runHook(cmd, payload, "beforePlan", mockRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v", result)
	}

	var got config.BeforePlanHookPayload

	err := json.Unmarshal(captured, &got)
	if err != nil {
		t.Fatalf("unmarshal stdin: %v", err)
	}

	if len(got.Hosts) != 2 || got.Hosts[0] != "server1" {
		t.Errorf("hosts = %v, want [server1 server2]", got.Hosts)
	}

	if got.WorkingDir != "/work" {
		t.Errorf("workingDir = %q, want /work", got.WorkingDir)
	}

	if got.Paths.ConfigPath != "config.yml" {
		t.Errorf("configPath = %q, want config.yml", got.Paths.ConfigPath)
	}
}

func TestRunHook_OutputForwarded(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "echo hello-hook"}

	result := runHook(cmd, "x", "test", defaultHookRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v", result)
	}

	output := out.String()
	if !strings.Contains(output, "hello-hook") {
		t.Errorf("expected hook output in writer, got %q", output)
	}
}

func TestRunHook_UsesPayloadBasePathAsWorkdir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	var out bytes.Buffer

	style := newOutputStyle(&out)
	cmd := &config.HookCommand{Command: "pwd"}
	payload := config.BeforeApplyPromptHookPayload{
		Hosts:      []string{"server1"},
		WorkingDir: "/should/not/be/used",
		Paths: config.HookConfigPaths{
			ConfigPath: "config.yml",
			BasePath:   tempDir,
		},
	}

	result := runHook(cmd, payload, "beforeApplyPrompt", defaultHookRunner, &out, style)
	if result != hookContinue {
		t.Fatalf("expected hookContinue, got %v; output: %s", result, out.String())
	}

	if !strings.Contains(out.String(), tempDir) {
		t.Fatalf("expected hook output to contain workdir %q, got %q", tempDir, out.String())
	}
}

func TestCollectHostNames(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{Host: config.HostEntry{Name: "alpha"}},
			{Host: config.HostEntry{Name: "beta"}},
		},
	}

	names := collectHostNames(plan)
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("collectHostNames = %v, want [alpha beta]", names)
	}
}
