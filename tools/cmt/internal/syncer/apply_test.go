package syncer

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/remote"

	"go.uber.org/mock/gomock"
)

func TestApplyWithDeps_Cancelled(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, false, false, &out, ApplyDependencies{
		ClientFactory: factory,
		Input:         strings.NewReader("n\n"),
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply cancelled.") {
		t.Fatalf("expected cancel output, got %q", out.String())
	}
}

func TestApplyWithDeps_UsesInjectedClientFactory(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:     "grafana",
						RemoteDir:       "/srv/grafana",
						PostSyncCommand: "echo done",
						Dirs: []DirPlan{
							{RelativePath: "data", RemotePath: "/srv/grafana/data", Exists: false, Action: ActionAdd},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
							{
								RelativePath: "old.txt",
								RemotePath:   "/srv/grafana/old.txt",
								Action:       ActionDelete,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/grafana/data").Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().Remove(gomock.Any(), "/srv/grafana/old.txt").Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "/srv/grafana", "echo done").Return("ok", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_BeforeApplyPromptHook_Rejected(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforeApplyPrompt: &config.HookCommand{Command: "reject"},
		},
	}

	mockRunner := func(_ context.Context, _ string, _ string, _ []byte) (int, string, error) {
		return 1, "rejected by policy", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply cancelled by hook.") {
		t.Fatalf("expected hook cancel output, got %q", output)
	}
}

func TestApplyWithDeps_BeforeApplyHook_Rejected(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforeApply: &config.HookCommand{Command: "reject"},
		},
	}

	mockRunner := func(_ context.Context, _ string, _ string, _ []byte) (int, string, error) {
		return 1, "", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply cancelled by hook.") {
		t.Fatalf("expected hook cancel output, got %q", output)
	}
}

func TestApplyWithDeps_BeforeApplyPromptHook_ErrorExitCode(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{RelativePath: "compose.yml", Action: ActionAdd, LocalPath: "/tmp/compose.yml", LocalData: []byte("x")},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforeApplyPrompt: &config.HookCommand{Command: "fail"},
		},
	}

	mockRunner := func(_ context.Context, _ string, _ string, _ []byte) (int, string, error) {
		return 2, "unexpected error", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err == nil {
		t.Fatal("expected error from hook exit code 2")
	}

	if err.Error() != "hook failed: beforeApplyPrompt" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyWithDeps_AllHooks_Pass(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	cfg := &config.CmtConfig{
		BeforeApplyHooks: &config.BeforeApplyHooks{
			BeforePlan:        &config.HookCommand{Command: "prepare-context"},
			BeforeApplyPrompt: &config.HookCommand{Command: "check-policy"},
			BeforeApply:       &config.HookCommand{Command: "final-gate"},
		},
	}

	hookCalls := 0
	mockRunner := func(_ context.Context, _ string, _ string, _ []byte) (int, string, error) {
		hookCalls++

		return 0, "", nil
	}

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), cfg, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
		HookRunner:    mockRunner,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if hookCalls != 3 {
		t.Fatalf("expected 3 hook calls, got %d", hookCalls)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_RefreshManifestOnNoop(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								Action:       ActionUnchanged,
								MaskHints: []MaskHint{
									{Prefix: "      - GF_SMTP_PASSWORD=", Suffix: ""},
								},
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().
			WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).
			Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, true, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "No changes to apply.") {
		t.Fatalf("expected noop output, got %q", output)
	}

	if !strings.Contains(output, "Manifest refreshed.") {
		t.Fatalf("expected manifest refreshed output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeUp(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "up",
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeStartServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "/srv/grafana", "docker compose up -d").Return("ok", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}

	if !strings.Contains(output, "compose") {
		t.Fatalf("expected compose output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeRecreate(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "up",
						RemoveOrphans: true,
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeRecreateServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionModify,
								LocalData:    []byte("services: {web: {}}"),
								RemoteData:   []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)
	composeCmd := "docker compose up -d --force-recreate --remove-orphans"

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", gomock.Any()).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "/srv/grafana", composeCmd).Return("ok", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}

	if !strings.Contains(output, "force-recreate") {
		t.Fatalf("expected force-recreate in compose output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeDown(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "down",
						Compose: &ComposePlan{
							DesiredAction: "down",
							ActionType:    ComposeStopServices,
							Services:      []string{"grafana"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "/srv/grafana", "docker compose down").Return("", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}
}

func TestApplyWithDeps_ComposeDownWithRemoveOrphans(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/srv/grafana",
						ComposeAction: "down",
						RemoveOrphans: true,
						Compose: &ComposePlan{
							DesiredAction: "down",
							ActionType:    ComposeStopServices,
							Services:      []string{"grafana"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "/srv/grafana", "docker compose down --remove-orphans").Return("", nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Apply complete!") {
		t.Fatalf("expected complete output, got %q", output)
	}
}

func TestProjectHasChanges_ComposeOnly(t *testing.T) {
	t.Parallel()

	projectPlan := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Compose: &ComposePlan{
			ActionType: ComposeStartServices,
			Services:   []string{"web"},
		},
	}

	if !projectHasChanges(projectPlan) {
		t.Error("projectHasChanges should return true when compose has changes")
	}
}

func TestProjectHasChanges_NoCompose(t *testing.T) {
	t.Parallel()

	projectPlan := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Compose: nil,
	}

	if projectHasChanges(projectPlan) {
		t.Error("projectHasChanges should return false when no changes")
	}
}

func TestProjectHasChanges_ComposeIgnore(t *testing.T) {
	t.Parallel()

	projectPlan := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		ComposeAction: config.ComposeActionIgnore,
		Compose: &ComposePlan{
			DesiredAction: config.ComposeActionIgnore,
			ActionType:    ComposeNoChange,
		},
	}

	if projectHasChanges(projectPlan) {
		t.Error("projectHasChanges should return false for composeAction=ignore")
	}
}

func TestApplyWithDeps_DirWithPermissionAndOwner(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:     "data",
								RemotePath:       "/srv/grafana/data",
								Exists:           false,
								Action:           ActionAdd,
								Permission:       "0755",
								Owner:            "app",
								Group:            "app",
								NeedsPermChange:  true,
								NeedsOwnerChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/grafana/data").Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chown app:app '/srv/grafana/data'").Return("", nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chmod 0755 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_DirPermissionOnly(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:    "data",
								RemotePath:      "/srv/grafana/data",
								Exists:          false,
								Action:          ActionAdd,
								Permission:      "0700",
								NeedsPermChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/grafana/data").Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chmod 0700 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_DirPermissionWithBecomeDefaultRoot(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:    "data",
								RemotePath:      "/srv/grafana/data",
								Exists:          false,
								Action:          ActionAdd,
								Permission:      "0700",
								NeedsPermChange: true,
								Become:          true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/grafana/data").Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "sudo -n chmod 0700 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_DirPermissionWithBecomeSpecificUser(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:    "data",
								RemotePath:      "/srv/grafana/data",
								Exists:          true,
								Action:          ActionModify,
								Permission:      "0750",
								NeedsPermChange: true,
								Become:          true,
								BecomeUser:      "ops",
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "sudo -n -u 'ops' chmod 0750 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_DirNoExtraCommands(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath: "data",
								RemotePath:   "/srv/grafana/data",
								Exists:       false,
								Action:       ActionAdd,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/grafana/data").Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_ExistingDirDriftIsReconciled(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:     "data",
								RemotePath:       "/srv/grafana/data",
								Exists:           true,
								Action:           ActionModify,
								Permission:       "0750",
								Owner:            "app",
								Group:            "app",
								ActualPermission: "755",
								ActualOwner:      "root",
								ActualGroup:      "root",
								NeedsPermChange:  true,
								NeedsOwnerChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chown app:app '/srv/grafana/data'").Return("", nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chmod 0750 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_DirRecursiveChown(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "snipeit",
						RemoteDir:   "/srv/snipeit",
						Dirs: []DirPlan{
							{
								RelativePath:     "redis_data",
								RemotePath:       "/srv/snipeit/redis_data",
								Exists:           false,
								Action:           ActionAdd,
								Owner:            "1000",
								Group:            "1000",
								Recursive:        true,
								Become:           true,
								NeedsOwnerChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/snipeit/compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().MkdirAll(gomock.Any(), "/srv/snipeit/redis_data").Return(nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "sudo -n chown -R 1000:1000 '/srv/snipeit/redis_data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/snipeit/compose.yml", []byte("services: {}")).Return(nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/snipeit/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestProjectHasChanges_ExistingDirMetadata(t *testing.T) {
	t.Parallel()

	projectPlan := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Dirs: []DirPlan{
			{
				RelativePath:     "data",
				RemotePath:       "/srv/grafana/data",
				Exists:           true,
				Action:           ActionModify,
				Permission:       "0750",
				ActualPermission: "755",
				NeedsPermChange:  true,
			},
		},
		Compose: nil,
	}

	if !projectHasChanges(projectPlan) {
		t.Error("projectHasChanges should return true when existing dir metadata must be reconciled")
	}
}

func TestProjectHasChanges_ExistingDirNoDrift(t *testing.T) {
	t.Parallel()

	projectPlan := ProjectPlan{
		Files: []FilePlan{
			{Action: ActionUnchanged},
		},
		Dirs: []DirPlan{
			{
				RelativePath: "data",
				RemotePath:   "/srv/grafana/data",
				Exists:       true,
				Action:       ActionUnchanged,
				Permission:   "0750",
			},
		},
		Compose: nil,
	}

	if projectHasChanges(projectPlan) {
		t.Error("projectHasChanges should return false when existing dir metadata matches")
	}
}

func TestApplyWithDeps_ExistingDirNoDrift(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath: "data",
								RemotePath:   "/srv/grafana/data",
								Exists:       true,
								Action:       ActionUnchanged,
								Permission:   "0750",
								Owner:        "app",
								Group:        "app",
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "No changes to apply.") {
		t.Fatalf("expected no changes output, got %q", out.String())
	}
}

func TestApplyWithDeps_ExistingDirPermissionDriftOnly(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:     "data",
								RemotePath:       "/srv/grafana/data",
								Exists:           true,
								Action:           ActionModify,
								Permission:       "0750",
								ActualPermission: "755",
								NeedsPermChange:  true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chmod 0750 '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}

func TestApplyWithDeps_ExistingDirOwnerDriftOnly(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "server1"},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/srv/grafana",
						Dirs: []DirPlan{
							{
								RelativePath:     "data",
								RemotePath:       "/srv/grafana/data",
								Exists:           true,
								Action:           ActionModify,
								Owner:            "app",
								Group:            "app",
								ActualOwner:      "root",
								ActualGroup:      "root",
								NeedsOwnerChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								LocalPath:    "/tmp/compose.yml",
								RemotePath:   "/srv/grafana/compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	ctrl := gomock.NewController(t)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	gomock.InOrder(
		factory.EXPECT().
			NewClient(config.HostEntry{Name: "server1"}).
			Return(client, nil),
		client.EXPECT().RunCommand(gomock.Any(), "", "chown app:app '/srv/grafana/data'").Return("", nil),
		client.EXPECT().WriteFile(gomock.Any(), "/srv/grafana/.cmt-manifest.json", gomock.Any()).Return(nil),
		client.EXPECT().Close().Return(nil),
	)

	var out bytes.Buffer

	err := ApplyWithDeps(context.Background(), &config.CmtConfig{}, plan, true, false, &out, ApplyDependencies{
		ClientFactory: factory,
	})
	if err != nil {
		t.Fatalf("ApplyWithDeps: %v", err)
	}

	if !strings.Contains(out.String(), "Apply complete!") {
		t.Fatalf("expected complete output, got %q", out.String())
	}
}
