package cmd

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/syncer"

	"github.com/spf13/cobra"
)

func TestVerifyExpectedPlanSHA256(t *testing.T) {
	t.Parallel()

	plan := &syncer.SyncPlan{
		HostPlans: []syncer.HostPlan{
			{
				Host: config.HostEntry{
					Name: "web",
					Host: "web.example.com",
					User: "deploy",
					Port: 22,
				},
				Projects: []syncer.ProjectPlan{
					{
						ProjectName: "app",
						RemoteDir:   "/srv/app",
						Files: []syncer.FilePlan{
							{
								RelativePath: "compose.yml",
								RemotePath:   "/srv/app/compose.yml",
								Action:       syncer.ActionAdd,
							},
						},
					},
				},
			},
		},
	}
	digest := syncer.PlanDigestSHA256(plan)

	tests := []struct {
		name           string
		expectedDigest string
		wantMismatch   bool
	}{
		{
			name:           "empty expectation allows apply",
			expectedDigest: "",
		},
		{
			name:           "matching digest allows apply",
			expectedDigest: digest,
		},
		{
			name:           "matching digest is case insensitive",
			expectedDigest: strings.ToUpper(digest),
		},
		{
			name:           "mismatched digest blocks apply",
			expectedDigest: strings.Repeat("0", len(digest)),
			wantMismatch:   true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			err := verifyExpectedPlanSHA256(plan, testCase.expectedDigest)
			if testCase.wantMismatch {
				if !errors.Is(err, errPlanDigestMismatch) {
					t.Fatalf("expected errPlanDigestMismatch, got %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("verifyExpectedPlanSHA256() returned error: %v", err)
			}
		})
	}
}

func TestPlanTargetFlagFiltersProjects(t *testing.T) {
	t.Parallel()

	var hostFilter []string

	var projectFilter []string

	var exitCode bool

	var digestFile string

	command := new(cobra.Command)
	bindPlanFlags(command, &hostFilter, &projectFilter, &exitCode, &digestFile)

	err := command.ParseFlags([]string{
		"--target", "grafana",
		"--project", "home-assistant",
		"--target=n8n",
	})
	if err != nil {
		t.Fatalf("ParseFlags() returned error: %v", err)
	}

	wantProjects := []string{"grafana", "home-assistant", "n8n"}
	if !reflect.DeepEqual(projectFilter, wantProjects) {
		t.Fatalf("projectFilter = %v, want %v", projectFilter, wantProjects)
	}
}

func TestApplyTargetFlagFiltersProjects(t *testing.T) {
	t.Parallel()

	var hostFilter []string

	var projectFilter []string

	var autoApprove bool

	var refreshManifestOnNoop bool

	var expectedPlanSHA256 string

	command := new(cobra.Command)
	bindApplyFlags(
		command,
		&hostFilter,
		&projectFilter,
		&autoApprove,
		&refreshManifestOnNoop,
		&expectedPlanSHA256,
	)

	err := command.ParseFlags([]string{"--target", "grafana", "--target=home-assistant"})
	if err != nil {
		t.Fatalf("ParseFlags() returned error: %v", err)
	}

	wantProjects := []string{"grafana", "home-assistant"}
	if !reflect.DeepEqual(projectFilter, wantProjects) {
		t.Fatalf("projectFilter = %v, want %v", projectFilter, wantProjects)
	}
}

func TestNormalizeTerraformTargetArgs(t *testing.T) {
	t.Parallel()

	args := normalizeTerraformTargetArgs([]string{
		"plan",
		"-target=grafana",
		"-target",
		"home-assistant",
		"--target",
		"n8n",
	})

	want := []string{
		"plan",
		"--target=grafana",
		"--target",
		"home-assistant",
		"--target",
		"n8n",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("normalized args = %v, want %v", args, want)
	}
}

func TestSharedStringSliceValueAccessors(t *testing.T) {
	t.Parallel()

	var values []string
	flagValue := newSharedStringSliceValue([]string{"default"}, &values)

	if flagValue.Type() != "stringSlice" {
		t.Fatalf("Type() = %q, want stringSlice", flagValue.Type())
	}

	if flagValue.String() != "[default]" {
		t.Fatalf("String() = %q, want [default]", flagValue.String())
	}

	err := flagValue.Set("grafana,home-assistant")
	if err != nil {
		t.Fatalf("Set() returned error: %v", err)
	}

	err = flagValue.Set("")
	if err != nil {
		t.Fatalf("Set() with empty value returned error: %v", err)
	}

	wantValues := []string{"grafana", "home-assistant"}
	if !reflect.DeepEqual(values, wantValues) {
		t.Fatalf("values = %v, want %v", values, wantValues)
	}

	if flagValue.String() != "[grafana,home-assistant]" {
		t.Fatalf("String() = %q, want [grafana,home-assistant]", flagValue.String())
	}
}

func TestExecuteNormalizesTerraformTargetArg(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	os.Args = []string{"cmt", "plan", "-target=grafana", "--help"}

	err := Execute()
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
}
