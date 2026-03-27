package syncer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cmt/internal/config"
	"cmt/internal/remote"

	"go.uber.org/mock/gomock"
)

var (
	errTestManifestNotFound  = errors.New("manifest not found")
	errTestRemoteFileMissing = errors.New("remote file missing")
	errTestNotFound          = errors.New("not found")
	errTestExitStatus1       = errors.New("exit status 1")
	errTestStatFailed        = errors.New("stat failed")
	errTestNotExist          = errors.New("not exist")
)

type mockLocalCommandRunner struct {
	run func(name string, args []string, workdir string) (string, error)
}

func (m mockLocalCommandRunner) Run(name string, args []string, workdir string) (string, error) {
	return m.run(name, args, workdir)
}

func TestCollectLocalFiles(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	setupCollectLocalFilesFixture(t, base)

	files, err := CollectLocalFiles(base, "server1", "grafana")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"compose.yml",
		"compose.override.yml",
		"grafana.ini",
		"provisioning/ds.yml",
	}

	assertContainsFiles(t, files, expected)

	// grafana.ini に対してホストの上書きが勝つことを確認します。
	data, _ := os.ReadFile(files["grafana.ini"])
	if string(data) != "[server]\nhost_override=true" {
		t.Errorf("grafana.ini should be host version, got %q", string(data))
	}
}

func setupCollectLocalFilesFixture(t *testing.T, base string) {
	t.Helper()

	projDir := filepath.Join(base, "projects", "grafana")
	mustMkdirAll(t, filepath.Join(projDir, "files", "provisioning"))
	mustWriteFile(t, filepath.Join(projDir, "compose.yml"), []byte("services: {}"))
	mustWriteFile(t, filepath.Join(projDir, "files", "grafana.ini"), []byte("[server]"))
	mustWriteFile(t, filepath.Join(projDir, "files", "provisioning", "ds.yml"), []byte("ds: 1"))

	hostDir := filepath.Join(base, "hosts", "server1", "grafana")
	mustMkdirAll(t, filepath.Join(hostDir, "files"))
	mustWriteFile(t, filepath.Join(hostDir, "compose.override.yml"), []byte("override: true"))
	mustWriteFile(t, filepath.Join(hostDir, "files", "grafana.ini"), []byte("[server]\nhost_override=true"))
}

func assertContainsFiles(t *testing.T, got map[string]string, expected []string) {
	t.Helper()

	if len(got) != len(expected) {
		t.Errorf("expected %d files, got %d: %v", len(expected), len(got), got)
	}

	for _, key := range expected {
		if _, ok := got[key]; !ok {
			t.Errorf("missing file %q", key)
		}
	}
}

func mustMkdirAll(t *testing.T, dir string) {
	t.Helper()

	err := os.MkdirAll(dir, 0750)
	if err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, filePath string, content []byte) {
	t.Helper()

	err := os.WriteFile(filePath, content, 0600)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectLocalFiles_MissingProject(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	err := os.MkdirAll(filepath.Join(base, "projects"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	files, err := CollectLocalFiles(base, "server1", "nonexistent")
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestBuildManifest(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"compose.yml": "/a/compose.yml",
		"config.ini":  "/a/config.ini",
	}

	manifest := BuildManifest(files)
	if len(manifest.ManagedFiles) != 2 {
		t.Errorf("expected 2 managed files, got %d", len(manifest.ManagedFiles))
	}

	if manifest.ManagedFiles[0] != "compose.yml" {
		t.Errorf("expected first file compose.yml, got %q", manifest.ManagedFiles[0])
	}
}

func TestIsBinary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "text should not be binary",
			data: []byte("hello world"),
			want: false,
		},
		{
			name: "data with null byte should be binary",
			data: []byte("hello\x00world"),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isBinary(tt.data)
			if got != tt.want {
				t.Errorf("isBinary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
	}
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			got := humanSize(testCase.n)
			if got != testCase.want {
				t.Errorf("humanSize(%d) = %q, want %q", testCase.n, got, testCase.want)
			}
		})
	}
}

func TestSyncPlanStats(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Files: []FilePlan{
							{Action: ActionAdd},
							{Action: ActionModify},
							{Action: ActionUnchanged},
						},
					},
					{
						Files: []FilePlan{
							{Action: ActionDelete},
							{Action: ActionAdd},
						},
					},
				},
			},
		},
	}

	hosts, projects, add, mod, del, unch := plan.Stats()
	if hosts != 1 {
		t.Errorf("hosts = %d", hosts)
	}

	if projects != 2 {
		t.Errorf("projects = %d", projects)
	}

	if add != 2 {
		t.Errorf("add = %d", add)
	}

	if mod != 1 {
		t.Errorf("mod = %d", mod)
	}

	if del != 1 {
		t.Errorf("del = %d", del)
	}

	if unch != 1 {
		t.Errorf("unch = %d", unch)
	}

	if !plan.HasChanges() {
		t.Error("plan should have changes")
	}
}

// ---------------------------------------------------------------------------
// ActionType helpers
// ---------------------------------------------------------------------------

func TestActionType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action ActionType
		want   string
	}{
		{ActionUnchanged, "unchanged"},
		{ActionAdd, "add"},
		{ActionModify, "modify"},
		{ActionDelete, "delete"},
	}
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			if got := testCase.action.String(); got != testCase.want {
				t.Errorf("String() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestActionType_Symbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		action ActionType
		want   string
	}{
		{ActionUnchanged, "="},
		{ActionAdd, "+"},
		{ActionModify, "~"},
		{ActionDelete, "-"},
	}
	for _, testCase := range tests {
		t.Run(testCase.want, func(t *testing.T) {
			t.Parallel()

			if got := testCase.action.Symbol(); got != testCase.want {
				t.Errorf("Symbol() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// computeDiff
// ---------------------------------------------------------------------------

func TestComputeDiff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		filename     string
		remote       []byte
		local        []byte
		wantEmpty    bool
		wantContains []string
	}{
		{
			name:      "identical content",
			filename:  "file.txt",
			remote:    []byte("hello\n"),
			local:     []byte("hello\n"),
			wantEmpty: true,
		},
		{
			name:     "basic diff",
			filename: "compose.yml",
			remote:   []byte("line1\nline2\nline3\n"),
			local:    []byte("line1\nmodified\nline3\n"),
			wantContains: []string{
				"compose.yml (remote)",
				"compose.yml (local)",
				"-line2",
				"+modified",
			},
		},
		{
			name:     "added lines",
			filename: "test.txt",
			remote:   []byte("a\n"),
			local:    []byte("a\nb\nc\n"),
			wantContains: []string{
				"+b",
				"+c",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := computeDiff(tt.filename, tt.remote, tt.local)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty diff, got %q", result)
				}

				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("diff should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// DirStats / HasChanges
// ---------------------------------------------------------------------------

func TestDirStats(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Dirs: []DirPlan{
							{RelativePath: "data", Exists: false, Action: ActionAdd},
							{RelativePath: "logs", Exists: true, Action: ActionUnchanged},
							{RelativePath: "config", Exists: false, Action: ActionAdd},
							{RelativePath: "cache", Exists: true, Action: ActionModify, NeedsPermChange: true},
						},
					},
				},
			},
		},
	}

	toCreate, toUpdate, existing := plan.DirStats()
	if toCreate != 2 {
		t.Errorf("toCreate = %d, want 2", toCreate)
	}

	if toUpdate != 1 {
		t.Errorf("toUpdate = %d, want 1", toUpdate)
	}

	if existing != 1 {
		t.Errorf("existing = %d, want 1", existing)
	}
}

func TestHasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		plan        *SyncPlan
		wantChanges bool
	}{
		{
			name: "no changes",
			plan: &SyncPlan{
				HostPlans: []HostPlan{
					{
						Projects: []ProjectPlan{
							{
								Files: []FilePlan{
									{Action: ActionUnchanged},
									{Action: ActionUnchanged},
								},
								Dirs: []DirPlan{
									{Exists: true, Action: ActionUnchanged},
								},
							},
						},
					},
				},
			},
			wantChanges: false,
		},
		{
			name: "dir creation",
			plan: &SyncPlan{
				HostPlans: []HostPlan{
					{
						Projects: []ProjectPlan{
							{
								Files: []FilePlan{
									{Action: ActionUnchanged},
								},
								Dirs: []DirPlan{
									{Exists: false, Action: ActionAdd},
								},
							},
						},
					},
				},
			},
			wantChanges: true,
		},
		{
			name: "existing dir metadata drift",
			plan: &SyncPlan{
				HostPlans: []HostPlan{
					{
						Projects: []ProjectPlan{
							{
								Files: []FilePlan{
									{Action: ActionUnchanged},
								},
								Dirs: []DirPlan{
									{Exists: true, Action: ActionModify, Permission: "0750", NeedsPermChange: true},
								},
							},
						},
					},
				},
			},
			wantChanges: true,
		},
		{
			name: "existing dir no drift",
			plan: &SyncPlan{
				HostPlans: []HostPlan{
					{
						Projects: []ProjectPlan{
							{
								Files: []FilePlan{
									{Action: ActionUnchanged},
								},
								Dirs: []DirPlan{
									{Exists: true, Action: ActionUnchanged, Permission: "0750"},
								},
							},
						},
					},
				},
			},
			wantChanges: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.plan.HasChanges()
			if got != tt.wantChanges {
				t.Errorf("HasChanges() = %v, want %v", got, tt.wantChanges)
			}
		})
	}
}

func TestSyncPlan_Print_NoHosts(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{}

	var buf bytes.Buffer

	plan.Print(&buf)

	if !strings.Contains(buf.String(), "No hosts selected") {
		t.Errorf("expected 'No hosts selected', got %q", buf.String())
	}
}

func TestSyncPlan_Print_UnchangedProjectCollapsed(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "srv", User: "u", Host: "h", Port: 22},
				Projects: []ProjectPlan{
					{
						ProjectName: "noop",
						RemoteDir:   "/opt/noop",
						Files:       []FilePlan{{RelativePath: "compose.yml", Action: ActionUnchanged}},
						Dirs:        []DirPlan{{RelativePath: "data", Action: ActionUnchanged}},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	plan.Print(&buf)
	output := buf.String()

	if !strings.Contains(output, "noop") {
		t.Error("output should contain project name")
	}

	if !strings.Contains(output, "(no changes)") {
		t.Error("unchanged project should be collapsed with (no changes)")
	}

	if strings.Contains(output, "Remote:") {
		t.Error("collapsed project should not show Remote: detail")
	}
}

func TestSyncPlan_Print_FullPlan(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{
					Name: "server1",
					User: "deploy",
					Host: "192.168.1.1",
					Port: 22,
				},
				Projects: []ProjectPlan{
					{
						ProjectName:     "grafana",
						RemoteDir:       "/opt/compose/grafana",
						PostSyncCommand: "echo done",
						Dirs: []DirPlan{
							{RelativePath: "data", Exists: false, Action: ActionAdd},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								Action:       ActionAdd,
								LocalData:    []byte("services: {}"),
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer

	plan.Print(&buf)
	output := buf.String()

	// ホストヘッダーをチェックします。
	if !strings.Contains(output, "Host: server1") {
		t.Error("output should contain host name")
	}

	if !strings.Contains(output, "deploy@192.168.1.1:22") {
		t.Error("output should contain connection info")
	}

	// プロジェクト情報をチェックします。
	if !strings.Contains(output, "Project: grafana") {
		t.Error("output should contain project name")
	}

	if !strings.Contains(output, "Post-sync:") {
		t.Error("output should contain post-sync command")
	}

	// ディレクトリプランをチェックします。
	if !strings.Contains(output, "+ data/") {
		t.Error("output should show dir to create")
	}

	// ファイルプランをチェックします。
	if !strings.Contains(output, "+ compose.yml") {
		t.Error("output should show added file")
	}

	// サマリーをチェックします。
	if !strings.Contains(output, "Summary:") {
		t.Error("output should contain summary")
	}

	if !strings.Contains(output, "1 to add") {
		t.Error("summary should show add count")
	}

	if !strings.Contains(output, "PROJECT") || !strings.Contains(output, "COMPOSE ACTION") ||
		!strings.Contains(output, "RESOURCES") {
		t.Error("output should contain per-host summary table header")
	}

	if !strings.Contains(output, "----------------------------------------------------------") {
		t.Error("output should contain summary table separator")
	}
}

func TestSyncPlan_Print_PerHostSummaryTable(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{Name: "srv-a", User: "u", Host: "a", Port: 22},
				Projects: []ProjectPlan{
					{
						ProjectName: "proj1",
						Files:       []FilePlan{{RelativePath: "compose.yml", Action: ActionAdd}},
						Compose:     &ComposePlan{ActionType: ComposeStartServices, Services: []string{"web"}},
					},
					{
						ProjectName: "proj2",
						Files:       []FilePlan{{RelativePath: "compose.yml", Action: ActionUnchanged}},
						Compose:     &ComposePlan{ActionType: ComposeNoChange},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	plan.Print(&buf)
	output := buf.String()

	if !strings.Contains(output, "Host: srv-a") {
		t.Error("summary should contain host name")
	}

	if !strings.Contains(output, "changed") || !strings.Contains(output, "unchanged") {
		t.Error("summary should show status (changed/unchanged)")
	}

	if !strings.Contains(output, "start (1)") {
		t.Error("summary should show compose action for proj1")
	}
}

func TestBuildPlanWithDeps_UsesInjectedDependencies(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/srv/compose"},
		Hosts: []config.HostEntry{
			{Name: "server1", Host: "server1-alias", User: "deploy"},
		},
	}

	ctrl := gomock.NewController(t)
	resolver := config.NewMockSSHConfigResolver(ctrl)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	hostDir := filepath.Join(base, "hosts", "server1")
	gomock.InOrder(
		resolver.EXPECT().
			Resolve(gomock.Any(), "", hostDir).
			DoAndReturn(func(entry *config.HostEntry, _, _ string) error {
				entry.Host = "10.0.0.10"
				entry.Port = 22

				return nil
			}),
		factory.EXPECT().
			NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).
			Return(client, nil),
	)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/.cmt-manifest.json").
		Return(nil, errTestManifestNotFound)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/compose.yml").
		Return(nil, errTestRemoteFileMissing)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose config --services 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose ps --services --filter status=running 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().Close().Return(nil)

	runner := mockLocalCommandRunner{
		run: func(name string, args []string, _ string) (string, error) {
			if name != "docker" {
				t.Fatalf("name = %q, want docker", name)
			}

			wantArgs := []string{"compose", "-f", "compose.yml", "config"}
			if len(args) != len(wantArgs) {
				t.Fatalf("args len = %d, want %d; args = %v", len(args), len(wantArgs), args)
			}

			for i := range wantArgs {
				if args[i] != wantArgs[i] {
					t.Fatalf("args[%d] = %q, want %q", i, args[i], wantArgs[i])
				}
			}

			return "ok", nil
		},
	}

	plan, err := BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner:   runner,
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps: %v", err)
	}

	if len(plan.HostPlans) != 1 {
		t.Fatalf("host plans = %d, want 1", len(plan.HostPlans))
	}

	if len(plan.HostPlans[0].Projects) != 1 {
		t.Fatalf("projects = %d, want 1", len(plan.HostPlans[0].Projects))
	}

	files := plan.HostPlans[0].Projects[0].Files
	if len(files) != 1 {
		t.Fatalf("files = %d, want 1", len(files))
	}

	if files[0].Action != ActionAdd {
		t.Fatalf("file action = %v, want %v", files[0].Action, ActionAdd)
	}
}

func TestBuildPlanWithDeps_ComposeValidationFails(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0o750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0o750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/srv/compose"},
		Hosts: []config.HostEntry{
			{Name: "server1", Host: "server1-alias", User: "deploy"},
		},
	}

	ctrl := gomock.NewController(t)
	resolver := config.NewMockSSHConfigResolver(ctrl)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	hostDir := filepath.Join(base, "hosts", "server1")
	gomock.InOrder(
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/.cmt-manifest.json").
		Return(nil, errTestManifestNotFound)
	client.EXPECT().
		ReadFile("/srv/compose/grafana/compose.yml").
		Return(nil, errTestRemoteFileMissing)
	client.EXPECT().Close().Return(nil)

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner: mockLocalCommandRunner{
			run: func(string, []string, string) (string, error) {
				return "compose is invalid", errTestExitStatus1
			},
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if !strings.Contains(err.Error(), "validating docker compose config for server1/grafana failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pattern-based diff masking
// ---------------------------------------------------------------------------

func TestBuildDiffMaskPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     string
		vars    map[string]any
		wantLen int
		wantPfx []string
		wantSfx []string
	}{
		{
			name:    "env var style",
			raw:     "DB_HOST={{ .db_host }}\nDB_PORT={{ .db_port }}\n",
			vars:    map[string]any{"db_host": "pg.local", "db_port": 5432},
			wantLen: 2,
			wantPfx: []string{"DB_HOST=", "DB_PORT="},
			wantSfx: []string{"", ""},
		},
		{
			name:    "value with surrounding text",
			raw:     `password = """{{ .pw }}"""` + "\n",
			vars:    map[string]any{"pw": "s3cret"},
			wantLen: 1,
			wantPfx: []string{`password = """`},
			wantSfx: []string{`"""`},
		},
		{
			name:    "no vars returns nil",
			raw:     "plain line\n",
			vars:    nil,
			wantLen: 0,
		},
		{
			name:    "no templates returns nil",
			raw:     "static content\n",
			vars:    map[string]any{"key": "val"},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rawData := []byte(tt.raw)

			rendered, err := RenderTemplate(rawData, tt.vars)
			if err != nil && tt.wantLen > 0 {
				t.Fatalf("render error: %v", err)
			}

			if tt.vars == nil {
				rendered = rawData
			}

			patterns := buildDiffMaskPatterns(rawData, rendered, tt.vars)
			if len(patterns) != tt.wantLen {
				t.Fatalf("len = %d, want %d; patterns = %+v", len(patterns), tt.wantLen, patterns)
			}

			for i, pat := range patterns {
				if i < len(tt.wantPfx) && pat.prefix != tt.wantPfx[i] {
					t.Errorf("pattern[%d].prefix = %q, want %q", i, pat.prefix, tt.wantPfx[i])
				}

				if i < len(tt.wantSfx) && pat.suffix != tt.wantSfx[i] {
					t.Errorf("pattern[%d].suffix = %q, want %q", i, pat.suffix, tt.wantSfx[i])
				}
			}
		})
	}
}

func TestMaskDiffWithPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		diff        string
		patterns    []maskPattern
		wantContain []string
		wantAbsent  []string
	}{
		{
			name: "masks + and - lines matching pattern",
			diff: "--- f (remote)\n+++ f (local)\n@@ -1,3 +1,3 @@\n" +
				" static line\n" +
				"-      - GF_SMTP_PASSWORD=old_secret\n" +
				"+      - GF_SMTP_PASSWORD=new_secret\n",
			patterns: []maskPattern{
				{prefix: "      - GF_SMTP_PASSWORD=", suffix: ""},
			},
			wantContain: []string{
				"--- f (remote)",
				"+++ f (local)",
				"@@ -1,3 +1,3 @@",
				" static line",
				"-      - GF_SMTP_PASSWORD=" + maskPlaceholder,
				"+      - GF_SMTP_PASSWORD=" + maskPlaceholder,
			},
			wantAbsent: []string{"old_secret", "new_secret"},
		},
		{
			name: "masks context lines too",
			diff: " static\n" +
				" HOST=secret_host\n" +
				"-PORT=old_port\n" +
				"+PORT=new_port\n",
			patterns: []maskPattern{
				{prefix: "HOST=", suffix: ""},
				{prefix: "PORT=", suffix: ""},
			},
			wantContain: []string{
				" static",
				" HOST=" + maskPlaceholder,
				"-PORT=" + maskPlaceholder,
				"+PORT=" + maskPlaceholder,
			},
			wantAbsent: []string{"secret_host", "old_port", "new_port"},
		},
		{
			name: "preserves suffix",
			diff: `-password = """old_pw"""` + "\n" +
				`+password = """new_pw"""` + "\n",
			patterns: []maskPattern{
				{prefix: `password = """`, suffix: `"""`},
			},
			wantContain: []string{
				`-password = """` + maskPlaceholder + `"""`,
				`+password = """` + maskPlaceholder + `"""`,
			},
			wantAbsent: []string{"old_pw", "new_pw"},
		},
		{
			name:        "no patterns returns diff unchanged",
			diff:        "+line1\n-line2\n",
			patterns:    nil,
			wantContain: []string{"+line1", "-line2"},
		},
		{
			name: "non-matching lines are preserved",
			diff: "+unrelated = hello\n+SECRET=hunter2\n",
			patterns: []maskPattern{
				{prefix: "SECRET=", suffix: ""},
			},
			wantContain: []string{"+unrelated = hello"},
			wantAbsent:  []string{"hunter2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := maskDiffWithPatterns(tt.diff, tt.patterns)

			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("result should contain %q, got:\n%s", want, got)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("result should NOT contain %q, got:\n%s", absent, got)
				}
			}
		})
	}
}

func TestApplyMaskToLine(t *testing.T) {
	t.Parallel()

	patterns := []maskPattern{
		{prefix: "      - GF_PASS=", suffix: ""},
		{prefix: `pw = """`, suffix: `"""`},
	}

	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "add line masked",
			line: "+      - GF_PASS=new_secret",
			want: "+      - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "remove line masked",
			line: "-      - GF_PASS=old_secret",
			want: "-      - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "context line masked",
			line: "       - GF_PASS=same_secret",
			want: "       - GF_PASS=" + maskPlaceholder,
		},
		{
			name: "suffix preserved",
			line: `+pw = """s3cret"""`,
			want: `+pw = """` + maskPlaceholder + `"""`,
		},
		{
			name: "header preserved",
			line: "--- file (remote)",
			want: "--- file (remote)",
		},
		{
			name: "hunk preserved",
			line: "@@ -1,3 +1,3 @@",
			want: "@@ -1,3 +1,3 @@",
		},
		{
			name: "non-matching line preserved",
			line: "+unrelated = hello",
			want: "+unrelated = hello",
		},
		{
			name: "empty line preserved",
			line: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := applyMaskToLine(tt.line, patterns)
			if got != tt.want {
				t.Errorf("applyMaskToLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestLongestCommonPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want string
	}{
		{"DB_HOST=pg.local", "DB_HOST=***", "DB_HOST="},
		{"abc", "abc", "abc"},
		{"abc", "xyz", ""},
		{"", "abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			t.Parallel()

			got := longestCommonPrefix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("longestCommonPrefix(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLongestCommonSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want string
	}{
		{`s3cret"""`, `***"""`, `"""`},
		{"abc", "abc", "abc"},
		{"abc", "xyz", ""},
		{"", "abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			t.Parallel()

			got := longestCommonSuffix(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("longestCommonSuffix(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestBuildMaskedVars(t *testing.T) {
	t.Parallel()

	vars := map[string]any{
		"key1": "value1",
		"key2": 42,
	}

	masked := buildMaskedVars(vars)

	if len(masked) != len(vars) {
		t.Fatalf("len = %d, want %d", len(masked), len(vars))
	}

	for k, v := range masked {
		if v != maskPlaceholder {
			t.Errorf("masked[%q] = %v, want %q", k, v, maskPlaceholder)
		}
	}
}

func TestBuildManifestWithMaskHints(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"compose.yml":          "/a/compose.yml",
		"compose.override.yml": "/a/compose.override.yml",
	}

	hints := map[string][]MaskHint{
		"compose.yml": {
			{Prefix: "GF_SMTP_PASSWORD=", Suffix: ""},
		},
		"deleted.yml": {
			{Prefix: "SHOULD_NOT_INCLUDE=", Suffix: ""},
		},
	}

	manifest := BuildManifestWithMaskHints(files, hints)

	if len(manifest.ManagedFiles) != 2 {
		t.Fatalf("managed files = %d, want 2", len(manifest.ManagedFiles))
	}

	if len(manifest.MaskHints) != 1 {
		t.Fatalf("mask hints files = %d, want 1", len(manifest.MaskHints))
	}

	composeHints, ok := manifest.MaskHints["compose.yml"]
	if !ok {
		t.Fatal("compose.yml mask hints should exist")
	}

	if len(composeHints) != 1 {
		t.Fatalf("compose.yml hints = %d, want 1", len(composeHints))
	}

	if composeHints[0].Prefix != "GF_SMTP_PASSWORD=" {
		t.Errorf("prefix = %q", composeHints[0].Prefix)
	}
}

func TestMaskDiffWithPatterns_UsesManifestHintsForRemovedSecrets(t *testing.T) {
	t.Parallel()

	diff := `--- compose.yml (remote)
+++ compose.yml (local)
@@ -1,4 +1,3 @@
       - GF_SERVER_ROOT_URL=https://grafana.shiron.dev/
-      - GF_SMTP_PASSWORD=old_secret_value
       - GF_SMTP_HOST=smtp.example.com:587
`

	patterns := mergeMaskPatterns(
		nil,
		patternsFromMaskHints([]MaskHint{
			{Prefix: "      - GF_SMTP_PASSWORD=", Suffix: ""},
		}),
	)

	masked := maskDiffWithPatterns(diff, patterns)

	if strings.Contains(masked, "old_secret_value") {
		t.Fatalf("removed secret should be masked:\n%s", masked)
	}

	if !strings.Contains(masked, "-      - GF_SMTP_PASSWORD="+maskPlaceholder) {
		t.Fatalf("masked removed line not found:\n%s", masked)
	}
}

// ---------------------------------------------------------------------------
// ComposePlan
// ---------------------------------------------------------------------------

func TestComposePlan_HasChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		plan *ComposePlan
		want bool
	}{
		{
			name: "nil plan",
			plan: nil,
			want: false,
		},
		{
			name: "no change",
			plan: &ComposePlan{ActionType: ComposeNoChange},
			want: false,
		},
		{
			name: "start with services",
			plan: &ComposePlan{
				ActionType: ComposeStartServices,
				Services:   []string{"web", "db"},
			},
			want: true,
		},
		{
			name: "stop with services",
			plan: &ComposePlan{
				ActionType: ComposeStopServices,
				Services:   []string{"web"},
			},
			want: true,
		},
		{
			name: "start but empty services",
			plan: &ComposePlan{
				ActionType: ComposeStartServices,
				Services:   nil,
			},
			want: false,
		},
		{
			name: "recreate with services",
			plan: &ComposePlan{
				ActionType: ComposeRecreateServices,
				Services:   []string{"web", "db"},
			},
			want: true,
		},
		{
			name: "ignore action keeps no changes",
			plan: &ComposePlan{
				DesiredAction: config.ComposeActionIgnore,
				ActionType:    ComposeNoChange,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.plan.HasChanges(); got != tt.want {
				t.Errorf("HasChanges() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildComposePlan_IgnoreAction(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	composePlan := buildComposePlan(config.ComposeActionIgnore, "/srv/compose/grafana", client, false)
	if composePlan == nil {
		t.Fatal("compose plan should not be nil")
	}

	if composePlan.DesiredAction != config.ComposeActionIgnore {
		t.Fatalf("DesiredAction = %q, want %q", composePlan.DesiredAction, config.ComposeActionIgnore)
	}

	if composePlan.HasChanges() {
		t.Fatal("ignore action should not produce compose state changes")
	}
}

func TestBuildComposePlan_UpWithFileChanges(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose config --services 2>/dev/null").
		Return("grafana\ninfluxdb\n", nil)
	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("grafana\ninfluxdb\n", nil)

	composePlan := buildComposePlan(config.ComposeActionUp, "/srv/compose/grafana", client, true)
	if composePlan == nil {
		t.Fatal("compose plan should not be nil")
	}

	if composePlan.ActionType != ComposeRecreateServices {
		t.Fatalf("ActionType = %v, want ComposeRecreateServices", composePlan.ActionType)
	}

	if len(composePlan.Services) != 2 {
		t.Fatalf("Services = %v, want 2 services", composePlan.Services)
	}

	if !composePlan.HasChanges() {
		t.Fatal("recreate should produce compose state changes")
	}
}

func TestBuildComposePlan_UpWithoutFileChanges(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose config --services 2>/dev/null").
		Return("grafana\ninfluxdb\n", nil)
	client.EXPECT().
		RunCommand("/srv/compose/grafana", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("grafana\ninfluxdb\n", nil)

	composePlan := buildComposePlan(config.ComposeActionUp, "/srv/compose/grafana", client, false)
	if composePlan == nil {
		t.Fatal("compose plan should not be nil")
	}

	if composePlan.ActionType != ComposeNoChange {
		t.Fatalf("ActionType = %v, want ComposeNoChange (all services running)", composePlan.ActionType)
	}

	if composePlan.HasChanges() {
		t.Fatal("no file changes + all running should not produce compose state changes")
	}
}

func TestHasChanges_ComposeOnly(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Files: []FilePlan{
							{Action: ActionUnchanged},
						},
						Dirs: []DirPlan{
							{Exists: true},
						},
						Compose: &ComposePlan{
							ActionType: ComposeStartServices,
							Services:   []string{"web"},
						},
					},
				},
			},
		},
	}

	if !plan.HasChanges() {
		t.Error("HasChanges() should return true when compose has changes")
	}
}

func TestComposeStats(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Compose: &ComposePlan{
							ActionType: ComposeStartServices,
							Services:   []string{"web", "db"},
						},
					},
					{
						Compose: &ComposePlan{
							ActionType: ComposeStopServices,
							Services:   []string{"redis"},
						},
					},
					{
						Compose: nil,
					},
				},
			},
		},
	}

	start, recreate, stop := plan.ComposeStats()
	if start != 2 {
		t.Errorf("start = %d, want 2", start)
	}

	if recreate != 0 {
		t.Errorf("recreate = %d, want 0", recreate)
	}

	if stop != 1 {
		t.Errorf("stop = %d, want 1", stop)
	}
}

func TestParseServiceLines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{
			name:   "normal output",
			output: "web\ndb\nredis\n",
			want:   []string{"db", "redis", "web"},
		},
		{
			name:   "empty output",
			output: "",
			want:   nil,
		},
		{
			name:   "whitespace only",
			output: "  \n  \n",
			want:   nil,
		},
		{
			name:   "with trailing whitespace",
			output: "web \n db\n",
			want:   []string{"db", "web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseServiceLines(tt.output)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got = %v", len(got), len(tt.want), got)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDiffServices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		all     []string
		running []string
		want    []string
	}{
		{
			name:    "some stopped",
			all:     []string{"db", "redis", "web"},
			running: []string{"web"},
			want:    []string{"db", "redis"},
		},
		{
			name:    "all running",
			all:     []string{"db", "web"},
			running: []string{"db", "web"},
			want:    nil,
		},
		{
			name:    "none running",
			all:     []string{"db", "web"},
			running: nil,
			want:    []string{"db", "web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := diffServices(tt.all, tt.running)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d; got = %v", len(got), len(tt.want), got)
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSyncPlan_Print_ComposeServices(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{
					Name: "server1",
					User: "deploy",
					Host: "192.168.1.1",
					Port: 22,
				},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/opt/compose/grafana",
						ComposeAction: "up",
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeStartServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer

	plan.Print(&buf)
	output := buf.String()

	if !strings.Contains(output, "Compose: up") {
		t.Error("output should contain compose action")
	}

	if !strings.Contains(output, "Compose services:") {
		t.Error("output should contain compose services header")
	}

	if !strings.Contains(output, "grafana") {
		t.Error("output should list grafana service")
	}

	if !strings.Contains(output, "influxdb") {
		t.Error("output should list influxdb service")
	}

	if !strings.Contains(output, "(start)") {
		t.Error("output should show (start) for up action")
	}

	if !strings.Contains(output, "service(s) to start") {
		t.Error("summary should mention services to start")
	}
}

func TestSyncPlan_Print_ComposeRecreateServices(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{
					Name: "server1",
					User: "deploy",
					Host: "192.168.1.1",
					Port: 22,
				},
				Projects: []ProjectPlan{
					{
						ProjectName:   "grafana",
						RemoteDir:     "/opt/compose/grafana",
						ComposeAction: "up",
						Compose: &ComposePlan{
							DesiredAction: "up",
							ActionType:    ComposeRecreateServices,
							Services:      []string{"grafana", "influxdb"},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
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

	var buf bytes.Buffer

	plan.Print(&buf)
	output := buf.String()

	if !strings.Contains(output, "(recreate)") {
		t.Error("output should show (recreate) for force-recreate action")
	}

	if !strings.Contains(output, "service(s) to recreate") {
		t.Error("summary should mention services to recreate")
	}
}

func TestComposeStats_WithRecreate(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Projects: []ProjectPlan{
					{
						Compose: &ComposePlan{
							ActionType: ComposeRecreateServices,
							Services:   []string{"web", "db"},
						},
					},
					{
						Compose: &ComposePlan{
							ActionType: ComposeStopServices,
							Services:   []string{"redis"},
						},
					},
				},
			},
		},
	}

	start, recreate, stop := plan.ComposeStats()
	if start != 0 {
		t.Errorf("start = %d, want 0", start)
	}

	if recreate != 2 {
		t.Errorf("recreate = %d, want 2", recreate)
	}

	if stop != 1 {
		t.Errorf("stop = %d, want 1", stop)
	}
}

func TestBuildPlanWithDeps_ProgressOutput(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/srv/compose"},
		Hosts: []config.HostEntry{
			{Name: "server1", Host: "server1-alias", User: "deploy"},
		},
	}

	ctrl := gomock.NewController(t)
	resolver := config.NewMockSSHConfigResolver(ctrl)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	hostDir := filepath.Join(base, "hosts", "server1")
	gomock.InOrder(
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().ReadFile("/srv/compose/grafana/.cmt-manifest.json").Return(nil, errTestNotFound)
	client.EXPECT().ReadFile("/srv/compose/grafana/compose.yml").Return(nil, errTestNotFound)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose config --services 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose ps --services --filter status=running 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().Close().Return(nil)

	var progressBuf bytes.Buffer

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory:  factory,
		SSHResolver:    resolver,
		LocalRunner:    mockLocalCommandRunner{run: func(string, []string, string) (string, error) { return "ok", nil }},
		ProgressWriter: &progressBuf,
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps: %v", err)
	}

	output := progressBuf.String()

	if !strings.Contains(output, "Planning:") {
		t.Error("progress should contain 'Planning:'")
	}

	if !strings.Contains(output, "1 host(s), 1 project(s)") {
		t.Errorf("progress should show host/project counts, got %q", output)
	}

	if !strings.Contains(output, "Planning host 1/1:") {
		t.Error("progress should contain host progress")
	}

	if !strings.Contains(output, "server1") {
		t.Error("progress should contain host name")
	}

	if !strings.Contains(output, "connecting...") {
		t.Error("progress should show connecting state")
	}

	if !strings.Contains(output, "project 1/1:") {
		t.Errorf("progress should contain project progress, got %q", output)
	}

	if !strings.Contains(output, "grafana") {
		t.Error("progress should contain project name")
	}

	if !strings.Contains(output, "done") {
		t.Error("progress should show done state")
	}

	if !strings.Contains(output, "Plan complete.") {
		t.Error("progress should show plan completion")
	}
}

func TestBuildPlanWithDeps_NoProgressWhenWriterNil(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	projectDir := filepath.Join(base, "projects", "grafana")

	err := os.MkdirAll(projectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(projectDir, "compose.yml"), []byte("services: {}"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/srv/compose"},
		Hosts: []config.HostEntry{
			{Name: "server1", Host: "server1-alias", User: "deploy"},
		},
	}

	ctrl := gomock.NewController(t)
	resolver := config.NewMockSSHConfigResolver(ctrl)
	factory := remote.NewMockClientFactory(ctrl)
	client := remote.NewMockRemoteClient(ctrl)

	hostDir := filepath.Join(base, "hosts", "server1")
	gomock.InOrder(
		resolver.EXPECT().Resolve(gomock.Any(), "", hostDir).Return(nil),
		factory.EXPECT().NewClient(gomock.AssignableToTypeOf(config.HostEntry{})).Return(client, nil),
	)
	client.EXPECT().ReadFile("/srv/compose/grafana/.cmt-manifest.json").Return(nil, errTestNotFound)
	client.EXPECT().ReadFile("/srv/compose/grafana/compose.yml").Return(nil, errTestNotFound)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose config --services 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().
		RunCommand(
			"/srv/compose/grafana",
			"docker compose ps --services --filter status=running 2>/dev/null",
		).
		Return("", errTestNotFound)
	client.EXPECT().Close().Return(nil)

	_, err = BuildPlanWithDeps(cfg, nil, nil, PlanDependencies{
		ClientFactory: factory,
		SSHResolver:   resolver,
		LocalRunner:   mockLocalCommandRunner{run: func(string, []string, string) (string, error) { return "ok", nil }},
	})
	if err != nil {
		t.Fatalf("BuildPlanWithDeps should succeed without progress writer: %v", err)
	}
}

func TestFormatDirPlanMeta(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		plan DirPlan
		want string
	}{
		{
			name: "no metadata on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd},
			want: "",
		},
		{
			name: "permission on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd, Permission: "0755", NeedsPermChange: true},
			want: " [mode=0755]",
		},
		{
			name: "owner on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd, Owner: "app", NeedsOwnerChange: true},
			want: " [owner=app]",
		},
		{
			name: "owner and group on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd, Owner: "app", Group: "staff", NeedsOwnerChange: true},
			want: " [owner=app:staff]",
		},
		{
			name: "group only on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd, Group: "staff", NeedsOwnerChange: true},
			want: " [owner=:staff]",
		},
		{
			name: "all fields on add",
			plan: DirPlan{RelativePath: "data", Action: ActionAdd, Permission: "0750", Owner: "app", Group: "app",
				NeedsPermChange: true, NeedsOwnerChange: true},
			want: " [mode=0750, owner=app:app]",
		},
		{
			name: "unchanged shows nothing",
			plan: DirPlan{RelativePath: "data", Action: ActionUnchanged, Permission: "0750", Owner: "app"},
			want: "",
		},
		{
			name: "permission drift on modify",
			plan: DirPlan{RelativePath: "data", Action: ActionModify, Permission: "0750",
				ActualPermission: "700", NeedsPermChange: true},
			want: " [mode: 700\u21920750]",
		},
		{
			name: "owner drift on modify",
			plan: DirPlan{RelativePath: "data", Action: ActionModify, Owner: "app", Group: "app",
				ActualOwner: "root", ActualGroup: "root", NeedsOwnerChange: true},
			want: " [owner: root:root\u2192app:app]",
		},
		{
			name: "all drift on modify",
			plan: DirPlan{RelativePath: "data", Action: ActionModify,
				Permission: "0750", Owner: "app", Group: "app",
				ActualPermission: "755", ActualOwner: "root", ActualGroup: "root",
				NeedsPermChange: true, NeedsOwnerChange: true},
			want: " [mode: 755\u21920750, owner: root:root\u2192app:app]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatDirPlanMeta(tt.plan)
			if got != tt.want {
				t.Errorf("formatDirPlanMeta() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSyncPlan_Print_DirMetadata(t *testing.T) {
	t.Parallel()

	plan := &SyncPlan{
		HostPlans: []HostPlan{
			{
				Host: config.HostEntry{
					Name: "server1",
					User: "deploy",
					Host: "192.168.1.1",
					Port: 22,
				},
				Projects: []ProjectPlan{
					{
						ProjectName: "grafana",
						RemoteDir:   "/opt/compose/grafana",
						Dirs: []DirPlan{
							{
								RelativePath: "data", Exists: false, Action: ActionAdd,
								Permission: "0755", Owner: "app", Group: "app",
								NeedsPermChange: true, NeedsOwnerChange: true,
							},
							{RelativePath: "logs", Exists: true, Action: ActionUnchanged},
							{
								RelativePath: "cache", Exists: true, Action: ActionModify,
								Permission: "0750", ActualPermission: "755",
								NeedsPermChange: true,
							},
						},
						Files: []FilePlan{
							{
								RelativePath: "compose.yml",
								Action:       ActionUnchanged,
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer

	plan.Print(&buf)
	output := buf.String()

	if !strings.Contains(output, "mode=0755") {
		t.Error("output should contain mode=0755 for new dir")
	}

	if !strings.Contains(output, "owner=app:app") {
		t.Error("output should contain owner=app:app for new dir")
	}

	if !strings.Contains(output, "data/") {
		t.Error("output should show data dir")
	}

	if !strings.Contains(output, "logs/") {
		t.Error("output should show logs dir")
	}

	if !strings.Contains(output, "(update)") {
		t.Error("output should show (update) for drifted dir")
	}

	if !strings.Contains(output, "755\u21920750") {
		t.Error("output should show permission drift arrow")
	}

	if !strings.Contains(output, "dir(s) to update") {
		t.Error("summary should mention dirs to update")
	}
}

func TestPermissionsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		desired string
		actual  string
		want    bool
	}{
		{"0755", "755", true},
		{"0700", "700", true},
		{"0750", "750", true},
		{"0755", "700", false},
		{"0750", "755", false},
		{"3755", "3755", true},
		{"0755", "3755", false},
		{"bad", "755", false},
		{"0755", "bad", false},
		{"same", "same", true},
	}

	for _, tt := range tests {
		t.Run(tt.desired+"_vs_"+tt.actual, func(t *testing.T) {
			t.Parallel()

			got := permissionsMatch(tt.desired, tt.actual)
			if got != tt.want {
				t.Errorf("permissionsMatch(%q, %q) = %v, want %v", tt.desired, tt.actual, got, tt.want)
			}
		})
	}
}

func TestComputeDirDrift_NoDrift(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().StatDirMetadata("/srv/data").Return(&remote.DirMetadata{
		Permission: "750",
		Owner:      "app",
		Group:      "app",
		OwnerID:    "1000",
		GroupID:    "1000",
	}, nil)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
		Permission: "0750",
		Owner:      "app",
		Group:      "app",
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionUnchanged {
		t.Errorf("Action = %v, want ActionUnchanged", plan.Action)
	}

	if plan.NeedsPermChange {
		t.Error("NeedsPermChange should be false")
	}

	if plan.NeedsOwnerChange {
		t.Error("NeedsOwnerChange should be false")
	}
}

func TestComputeDirDrift_OwnerGroupNoDrift_WhenDesiredUsesIDs(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().StatDirMetadata("/srv/data").Return(&remote.DirMetadata{
		Permission: "750",
		Owner:      "opc",
		Group:      "opc",
		OwnerID:    "1000",
		GroupID:    "1000",
	}, nil)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
		Permission: "0750",
		Owner:      "1000",
		Group:      "1000",
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionUnchanged {
		t.Errorf("Action = %v, want ActionUnchanged", plan.Action)
	}

	if plan.NeedsPermChange {
		t.Error("NeedsPermChange should be false")
	}

	if plan.NeedsOwnerChange {
		t.Error("NeedsOwnerChange should be false")
	}
}

func TestComputeDirDrift_PermissionDrift(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().StatDirMetadata("/srv/data").Return(&remote.DirMetadata{
		Permission: "700",
		Owner:      "app",
		Group:      "app",
	}, nil)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
		Permission: "0750",
		Owner:      "app",
		Group:      "app",
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionModify {
		t.Errorf("Action = %v, want ActionModify", plan.Action)
	}

	if !plan.NeedsPermChange {
		t.Error("NeedsPermChange should be true")
	}

	if plan.NeedsOwnerChange {
		t.Error("NeedsOwnerChange should be false")
	}
}

func TestComputeDirDrift_OwnerDrift(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().StatDirMetadata("/srv/data").Return(&remote.DirMetadata{
		Permission: "750",
		Owner:      "root",
		Group:      "root",
	}, nil)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
		Permission: "0750",
		Owner:      "app",
		Group:      "app",
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionModify {
		t.Errorf("Action = %v, want ActionModify", plan.Action)
	}

	if plan.NeedsPermChange {
		t.Error("NeedsPermChange should be false")
	}

	if !plan.NeedsOwnerChange {
		t.Error("NeedsOwnerChange should be true")
	}
}

func TestComputeDirDrift_NoDesiredMetadata(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionUnchanged {
		t.Errorf("Action = %v, want ActionUnchanged", plan.Action)
	}
}

func TestComputeDirDrift_StatError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().StatDirMetadata("/srv/data").Return(nil, errTestStatFailed)

	plan := &DirPlan{
		RemotePath: "/srv/data",
		Exists:     true,
		Permission: "0750",
		Owner:      "app",
	}

	computeDirDrift(plan, client)

	if plan.Action != ActionModify {
		t.Errorf("Action = %v, want ActionModify (assume drift on error)", plan.Action)
	}

	if !plan.NeedsPermChange {
		t.Error("NeedsPermChange should be true on error")
	}

	if !plan.NeedsOwnerChange {
		t.Error("NeedsOwnerChange should be true on error")
	}
}

func TestBuildDirPlans_WithDriftDetection(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().Stat("/srv/compose/new_dir").Return(nil, errTestNotExist)
	client.EXPECT().Stat("/srv/compose/existing_ok").Return(nil, nil)
	client.EXPECT().StatDirMetadata("/srv/compose/existing_ok").Return(&remote.DirMetadata{
		Permission: "750",
		Owner:      "app",
		Group:      "app",
	}, nil)
	client.EXPECT().Stat("/srv/compose/existing_drift").Return(nil, nil)
	client.EXPECT().StatDirMetadata("/srv/compose/existing_drift").Return(&remote.DirMetadata{
		Permission: "700",
		Owner:      "root",
		Group:      "root",
	}, nil)

	dirs := []config.DirConfig{
		{Path: "new_dir", Permission: "0755", Owner: "app", Group: "app"},
		{Path: "existing_ok", Permission: "0750", Owner: "app", Group: "app"},
		{Path: "existing_drift", Permission: "0750", Owner: "app", Group: "app"},
	}

	plans := buildDirPlans(dirs, "/srv/compose", client)

	if len(plans) != 3 {
		t.Fatalf("len = %d, want 3", len(plans))
	}

	if plans[0].Action != ActionAdd {
		t.Errorf("new_dir: Action = %v, want ActionAdd", plans[0].Action)
	}

	if !plans[0].NeedsPermChange || !plans[0].NeedsOwnerChange {
		t.Error("new_dir should need both perm and owner changes")
	}

	if plans[1].Action != ActionUnchanged {
		t.Errorf("existing_ok: Action = %v, want ActionUnchanged", plans[1].Action)
	}

	if plans[2].Action != ActionModify {
		t.Errorf("existing_drift: Action = %v, want ActionModify", plans[2].Action)
	}

	if !plans[2].NeedsPermChange || !plans[2].NeedsOwnerChange {
		t.Error("existing_drift should need both perm and owner changes")
	}

	if plans[2].ActualPermission != "700" {
		t.Errorf("existing_drift: ActualPermission = %q, want %q", plans[2].ActualPermission, "700")
	}
}

func TestBuildDirPlans_Recursive(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().Stat("/srv/compose/redis_data").Return(nil, nil)
	client.EXPECT().StatDirMetadata("/srv/compose/redis_data").Return(&remote.DirMetadata{
		Permission: "755",
		Owner:      "1000",
		Group:      "1000",
	}, nil)

	dirs := []config.DirConfig{
		{Path: "redis_data", Owner: "1000", Group: "1000", Recursive: true, Become: true},
	}

	plans := buildDirPlans(dirs, "/srv/compose", client)

	if len(plans) != 1 {
		t.Fatalf("len = %d, want 1", len(plans))
	}

	if !plans[0].Recursive {
		t.Error("Recursive = false, want true")
	}

	if plans[0].Owner != "1000" || plans[0].Group != "1000" {
		t.Errorf("Owner = %q, Group = %q", plans[0].Owner, plans[0].Group)
	}
}

// ---------------------------------------------------------------------------
// buildDeleteFilePlans
// ---------------------------------------------------------------------------

func TestBuildDeleteFilePlans_NilManifest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	plans := buildDeleteFilePlans(nil, map[string]bool{}, "/srv/grafana", client)
	if len(plans) != 0 {
		t.Errorf("expected no plans for nil manifest, got %d", len(plans))
	}
}

func TestBuildDeleteFilePlans_ManagedFileGone(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	manifest := &Manifest{
		ManagedFiles: []string{"old.txt", "compose.yml"},
	}
	// old.txt はローカルにないが compose.yml は残っている
	localSet := map[string]bool{"compose.yml": true}

	client.EXPECT().ReadFile("/srv/grafana/old.txt").Return([]byte("old content"), nil)

	plans := buildDeleteFilePlans(manifest, localSet, "/srv/grafana", client)

	if len(plans) != 1 {
		t.Fatalf("expected 1 delete plan, got %d", len(plans))
	}

	if plans[0].RelativePath != "old.txt" {
		t.Errorf("RelativePath = %q, want %q", plans[0].RelativePath, "old.txt")
	}

	if plans[0].Action != ActionDelete {
		t.Errorf("Action = %v, want ActionDelete", plans[0].Action)
	}

	if string(plans[0].RemoteData) != "old content" {
		t.Errorf("RemoteData = %q, want %q", string(plans[0].RemoteData), "old content")
	}
}

func TestBuildDeleteFilePlans_ManifestFileExcluded(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	// manifestFile自体はmanifest.ManagedFilesに入ることはないが、
	// 念のため含まれていても削除対象にならないことを確認します。
	manifest := &Manifest{
		ManagedFiles: []string{manifestFile},
	}

	plans := buildDeleteFilePlans(manifest, map[string]bool{}, "/srv/grafana", client)
	if len(plans) != 0 {
		t.Errorf("manifest file should not appear as delete plan, got %d plans", len(plans))
	}
}

// ---------------------------------------------------------------------------
// readManifest
// ---------------------------------------------------------------------------

func TestReadManifest_NotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().ReadFile("/srv/grafana/.cmt-manifest.json").Return(nil, errTestManifestNotFound)

	manifest := readManifest(client, "/srv/grafana")
	if manifest != nil {
		t.Errorf("expected nil manifest when file not found, got %+v", manifest)
	}
}

func TestReadManifest_InvalidJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().ReadFile("/srv/grafana/.cmt-manifest.json").Return([]byte("not json {{{"), nil)

	manifest := readManifest(client, "/srv/grafana")
	if manifest != nil {
		t.Errorf("expected nil manifest on invalid JSON, got %+v", manifest)
	}
}

func TestReadManifest_Valid(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	jsonData := `{"managedFiles":["compose.yml","config.ini"]}`
	client.EXPECT().ReadFile("/srv/grafana/.cmt-manifest.json").Return([]byte(jsonData), nil)

	manifest := readManifest(client, "/srv/grafana")
	if manifest == nil {
		t.Fatal("expected non-nil manifest")
	}

	if len(manifest.ManagedFiles) != 2 {
		t.Errorf("ManagedFiles len = %d, want 2", len(manifest.ManagedFiles))
	}

	if manifest.ManagedFiles[0] != "compose.yml" {
		t.Errorf("ManagedFiles[0] = %q, want %q", manifest.ManagedFiles[0], "compose.yml")
	}
}

// ---------------------------------------------------------------------------
// maskHintsFromManifest
// ---------------------------------------------------------------------------

func TestMaskHintsFromManifest_NilManifest(t *testing.T) {
	t.Parallel()

	hints := maskHintsFromManifest(nil, "compose.yml")
	if hints != nil {
		t.Errorf("expected nil for nil manifest, got %v", hints)
	}
}

func TestMaskHintsFromManifest_NoMaskHints(t *testing.T) {
	t.Parallel()

	manifest := &Manifest{ManagedFiles: []string{"compose.yml"}}
	hints := maskHintsFromManifest(manifest, "compose.yml")

	if hints != nil {
		t.Errorf("expected nil for manifest with no MaskHints, got %v", hints)
	}
}

func TestMaskHintsFromManifest_KeyNotFound(t *testing.T) {
	t.Parallel()

	manifest := &Manifest{
		ManagedFiles: []string{"compose.yml"},
		MaskHints: map[string][]MaskHint{
			"other.yml": {{Prefix: "pass=", Suffix: ""}},
		},
	}

	hints := maskHintsFromManifest(manifest, "compose.yml")
	if hints != nil {
		t.Errorf("expected nil when key not found, got %v", hints)
	}
}

func TestMaskHintsFromManifest_KeyFound(t *testing.T) {
	t.Parallel()

	manifest := &Manifest{
		ManagedFiles: []string{"compose.yml"},
		MaskHints: map[string][]MaskHint{
			"compose.yml": {{Prefix: "PASSWORD=", Suffix: ""}},
		},
	}

	hints := maskHintsFromManifest(manifest, "compose.yml")
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}

	if hints[0].Prefix != "PASSWORD=" {
		t.Errorf("Prefix = %q, want %q", hints[0].Prefix, "PASSWORD=")
	}

	// 元のスライスとは独立したコピーであることを確認します。
	hints[0].Prefix = "MODIFIED="
	if manifest.MaskHints["compose.yml"][0].Prefix != "PASSWORD=" {
		t.Error("maskHintsFromManifest should return a copy, not a reference")
	}
}

// ---------------------------------------------------------------------------
// maskHintsFromPatterns
// ---------------------------------------------------------------------------

func TestMaskHintsFromPatterns_Empty(t *testing.T) {
	t.Parallel()

	hints := maskHintsFromPatterns(nil)
	if hints != nil {
		t.Errorf("expected nil for empty patterns, got %v", hints)
	}

	hints = maskHintsFromPatterns([]maskPattern{})
	if hints != nil {
		t.Errorf("expected nil for empty slice, got %v", hints)
	}
}

func TestMaskHintsFromPatterns_NonEmpty(t *testing.T) {
	t.Parallel()

	patterns := []maskPattern{
		{prefix: "password=", suffix: ""},
		{prefix: "key=", suffix: " #secret"},
	}

	hints := maskHintsFromPatterns(patterns)
	if len(hints) != 2 {
		t.Fatalf("expected 2 hints, got %d", len(hints))
	}

	if hints[0].Prefix != "password=" {
		t.Errorf("hints[0].Prefix = %q, want %q", hints[0].Prefix, "password=")
	}

	if hints[1].Suffix != " #secret" {
		t.Errorf("hints[1].Suffix = %q, want %q", hints[1].Suffix, " #secret")
	}
}

// ---------------------------------------------------------------------------
// mergeMaskPatterns
// ---------------------------------------------------------------------------

func TestMergeMaskPatterns_EmptyPrimary(t *testing.T) {
	t.Parallel()

	secondary := []maskPattern{{prefix: "a=", suffix: ""}}
	merged := mergeMaskPatterns(nil, secondary)

	if len(merged) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(merged))
	}

	if merged[0].prefix != "a=" {
		t.Errorf("merged[0].prefix = %q, want %q", merged[0].prefix, "a=")
	}
}

func TestMergeMaskPatterns_EmptySecondary(t *testing.T) {
	t.Parallel()

	primary := []maskPattern{{prefix: "b=", suffix: ""}}
	merged := mergeMaskPatterns(primary, nil)

	if len(merged) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(merged))
	}

	if merged[0].prefix != "b=" {
		t.Errorf("merged[0].prefix = %q, want %q", merged[0].prefix, "b=")
	}
}

func TestMergeMaskPatterns_DeduplicatesSamePattern(t *testing.T) {
	t.Parallel()

	primary := []maskPattern{{prefix: "p=", suffix: ""}}
	secondary := []maskPattern{{prefix: "p=", suffix: ""}} // 同じパターン

	merged := mergeMaskPatterns(primary, secondary)
	if len(merged) != 1 {
		t.Errorf("duplicate patterns should be deduplicated, got %d patterns", len(merged))
	}
}

func TestMergeMaskPatterns_MergesDistinct(t *testing.T) {
	t.Parallel()

	primary := []maskPattern{{prefix: "a=", suffix: ""}}
	secondary := []maskPattern{
		{prefix: "a=", suffix: ""}, // duplicate → skip
		{prefix: "b=", suffix: ""}, // new
	}

	merged := mergeMaskPatterns(primary, secondary)
	if len(merged) != 2 {
		t.Errorf("expected 2 merged patterns, got %d: %v", len(merged), merged)
	}
}

// ---------------------------------------------------------------------------
// projectFilesOrDirsChanged
// ---------------------------------------------------------------------------

func TestProjectFilesOrDirsChanged_AllUnchanged(t *testing.T) {
	t.Parallel()

	filePlans := []FilePlan{
		{Action: ActionUnchanged},
		{Action: ActionUnchanged},
	}
	dirPlans := []DirPlan{
		{Action: ActionUnchanged},
	}

	if projectFilesOrDirsChanged(filePlans, dirPlans) {
		t.Error("expected false when all unchanged")
	}
}

func TestProjectFilesOrDirsChanged_FileChanged(t *testing.T) {
	t.Parallel()

	filePlans := []FilePlan{
		{Action: ActionUnchanged},
		{Action: ActionModify},
	}
	dirPlans := []DirPlan{}

	if !projectFilesOrDirsChanged(filePlans, dirPlans) {
		t.Error("expected true when a file is modified")
	}
}

func TestProjectFilesOrDirsChanged_FileAdded(t *testing.T) {
	t.Parallel()

	filePlans := []FilePlan{{Action: ActionAdd}}
	dirPlans := []DirPlan{}

	if !projectFilesOrDirsChanged(filePlans, dirPlans) {
		t.Error("expected true when a file is added")
	}
}

func TestProjectFilesOrDirsChanged_DirChanged(t *testing.T) {
	t.Parallel()

	filePlans := []FilePlan{{Action: ActionUnchanged}}
	dirPlans := []DirPlan{{Action: ActionAdd}}

	if !projectFilesOrDirsChanged(filePlans, dirPlans) {
		t.Error("expected true when a dir is added")
	}
}

func TestProjectFilesOrDirsChanged_Empty(t *testing.T) {
	t.Parallel()

	if projectFilesOrDirsChanged(nil, nil) {
		t.Error("expected false for empty slices")
	}
}

// ---------------------------------------------------------------------------
// buildLocalFilePlan
// ---------------------------------------------------------------------------

func TestBuildLocalFilePlan_NewFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	localPath := base + "/compose.yml"
	mustWriteFile(t, localPath, []byte("services: {}"))

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)
	client.EXPECT().ReadFile("/srv/grafana/compose.yml").Return(nil, errTestRemoteFileMissing)

	plan, err := buildLocalFilePlan(
		"compose.yml", localPath, "/srv/grafana", client, nil, nil,
	)
	if err != nil {
		t.Fatalf("buildLocalFilePlan: %v", err)
	}

	if plan.Action != ActionAdd {
		t.Errorf("Action = %v, want ActionAdd", plan.Action)
	}

	if string(plan.LocalData) != "services: {}" {
		t.Errorf("LocalData = %q, want %q", string(plan.LocalData), "services: {}")
	}

	if plan.RemotePath != "/srv/grafana/compose.yml" {
		t.Errorf("RemotePath = %q, want %q", plan.RemotePath, "/srv/grafana/compose.yml")
	}
}

func TestBuildLocalFilePlan_Unchanged(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	localPath := base + "/compose.yml"
	content := []byte("services: {}")
	mustWriteFile(t, localPath, content)

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)
	// リモートも同じ内容
	client.EXPECT().ReadFile("/srv/grafana/compose.yml").Return(content, nil)

	plan, err := buildLocalFilePlan(
		"compose.yml", localPath, "/srv/grafana", client, nil, nil,
	)
	if err != nil {
		t.Fatalf("buildLocalFilePlan: %v", err)
	}

	if plan.Action != ActionUnchanged {
		t.Errorf("Action = %v, want ActionUnchanged", plan.Action)
	}
}

func TestBuildLocalFilePlan_Modified(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	localPath := base + "/compose.yml"
	localContent := []byte("services:\n  web:\n    image: nginx:latest\n")
	remoteContent := []byte("services:\n  web:\n    image: nginx:old\n")
	mustWriteFile(t, localPath, localContent)

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)
	client.EXPECT().ReadFile("/srv/grafana/compose.yml").Return(remoteContent, nil)

	plan, err := buildLocalFilePlan(
		"compose.yml", localPath, "/srv/grafana", client, nil, nil,
	)
	if err != nil {
		t.Fatalf("buildLocalFilePlan: %v", err)
	}

	if plan.Action != ActionModify {
		t.Errorf("Action = %v, want ActionModify", plan.Action)
	}

	if plan.Diff == "" {
		t.Error("Diff should not be empty for modified file")
	}

	if !strings.Contains(plan.Diff, "-") || !strings.Contains(plan.Diff, "+") {
		t.Errorf("Diff should contain +/- markers, got: %s", plan.Diff)
	}
}

func TestBuildLocalFilePlan_WithMaskHints(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	localPath := base + "/config.env"
	content := []byte("DB_PASSWORD=supersecret\n")
	mustWriteFile(t, localPath, content)

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)
	client.EXPECT().ReadFile("/srv/app/config.env").Return([]byte("DB_PASSWORD=old_secret\n"), nil)

	hints := []MaskHint{{Prefix: "DB_PASSWORD=", Suffix: ""}}

	plan, err := buildLocalFilePlan(
		"config.env", localPath, "/srv/app", client, nil, hints,
	)
	if err != nil {
		t.Fatalf("buildLocalFilePlan: %v", err)
	}

	if plan.Action != ActionModify {
		t.Errorf("Action = %v, want ActionModify", plan.Action)
	}

	// マスクヒントが結果に保存されていることを確認します。
	if len(plan.MaskHints) == 0 {
		t.Error("MaskHints should be propagated to the plan")
	}
}

// ---------------------------------------------------------------------------
// buildComposePlan - 未テストの分岐
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// maskDiffWithPatterns - trim branch (diff without trailing newline)
// ---------------------------------------------------------------------------

func TestMaskDiffWithPatterns_DiffWithoutTrailingNewline(t *testing.T) {
	t.Parallel()

	// 末尾に改行がないdiffでトリム分岐をカバーします。
	diff := "+SECRET=old_value\n-SECRET=new_value" // 末尾に改行なし
	patterns := []maskPattern{{prefix: "SECRET=", suffix: ""}}

	got := maskDiffWithPatterns(diff, patterns)

	// マスクが適用されていることを確認します。
	if strings.Contains(got, "old_value") || strings.Contains(got, "new_value") {
		t.Errorf("secrets should be masked, got: %s", got)
	}

	if !strings.Contains(got, maskPlaceholder) {
		t.Errorf("result should contain mask placeholder, got: %s", got)
	}

	// 末尾に余分な改行がないことを確認します。
	if strings.HasSuffix(got, "\n") {
		t.Errorf("result should not end with newline when input doesn't, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// matchesPattern - suffix mismatch branch
// ---------------------------------------------------------------------------

func TestMatchesPattern_SuffixMismatch(t *testing.T) {
	t.Parallel()

	pat := maskPattern{prefix: `pw = """`, suffix: `"""`}

	// suffixが一致しない場合はfalseを返すべきです。
	if matchesPattern(`pw = """secret`, pat) {
		t.Error("matchesPattern should return false when suffix doesn't match")
	}
}

func TestMatchesPattern_NoSuffix(t *testing.T) {
	t.Parallel()

	pat := maskPattern{prefix: "SECRET=", suffix: ""}

	// suffixなしで一致する場合はtrueを返すべきです。
	if !matchesPattern("SECRET=abc123", pat) {
		t.Error("matchesPattern should return true when prefix matches and no suffix required")
	}
}

// ---------------------------------------------------------------------------

func TestBuildComposePlan_DownWithRunningServices(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().
		RunCommand("/srv/grafana", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("grafana\ninfluxdb\n", nil)

	plan := buildComposePlan(config.ComposeActionDown, "/srv/grafana", client, false)
	if plan == nil {
		t.Fatal("plan should not be nil")
	}

	if plan.ActionType != ComposeStopServices {
		t.Errorf("ActionType = %v, want ComposeStopServices", plan.ActionType)
	}

	if len(plan.Services) != 2 {
		t.Errorf("Services = %v, want 2 services", plan.Services)
	}
}

func TestBuildComposePlan_DownWithNoRunningServices(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	client.EXPECT().
		RunCommand("/srv/grafana", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("", nil)

	plan := buildComposePlan(config.ComposeActionDown, "/srv/grafana", client, false)
	if plan == nil {
		t.Fatal("plan should not be nil")
	}

	if plan.ActionType != ComposeNoChange {
		t.Errorf("ActionType = %v, want ComposeNoChange", plan.ActionType)
	}

	if plan.HasChanges() {
		t.Error("down with no running services should not have changes")
	}
}

func TestBuildComposePlan_UpWithSomeServicesStopped(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	// defined: web, db, cache / running: web, db → cache が止まっている
	client.EXPECT().
		RunCommand("/srv/app", "docker compose config --services 2>/dev/null").
		Return("web\ndb\ncache\n", nil)
	client.EXPECT().
		RunCommand("/srv/app", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("web\ndb\n", nil)

	plan := buildComposePlan(config.ComposeActionUp, "/srv/app", client, false)
	if plan == nil {
		t.Fatal("plan should not be nil")
	}

	if plan.ActionType != ComposeStartServices {
		t.Errorf("ActionType = %v, want ComposeStartServices", plan.ActionType)
	}

	if len(plan.Services) != 1 || plan.Services[0] != "cache" {
		t.Errorf("Services = %v, want [cache]", plan.Services)
	}
}

// ---------------------------------------------------------------------------
// printFileDiff
// ---------------------------------------------------------------------------

func TestPrintFileDiff_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printFileDiff(&buf, outputStyle{}, "")

	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty diff, got %q", buf.String())
	}
}

func TestPrintFileDiff_NonEmpty(t *testing.T) {
	t.Parallel()

	diff := "--- a.yml (remote)\n+++ a.yml (local)\n@@ -1 +1 @@\n-old\n+new\n"

	var buf bytes.Buffer
	printFileDiff(&buf, outputStyle{enabled: false}, diff)

	output := buf.String()
	if !strings.Contains(output, "old") {
		t.Errorf("output should contain 'old', got %q", output)
	}

	if !strings.Contains(output, "new") {
		t.Errorf("output should contain 'new', got %q", output)
	}
}

// ---------------------------------------------------------------------------
// outputStyle.diffLine
// ---------------------------------------------------------------------------

func TestOutputStyle_DiffLine(t *testing.T) {
	t.Parallel()

	style := outputStyle{enabled: false}

	tests := []struct {
		line string
	}{
		{"--- a.yml (remote)"},
		{"+++ a.yml (local)"},
		{"@@ -1 +1 @@"},
		{"+added line"},
		{"-removed line"},
		{" context line"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()

			// カラーなしモードではそのまま返る
			got := style.diffLine(tt.line)
			if got != tt.line {
				t.Errorf("diffLine(%q) = %q, want same (no color)", tt.line, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// shouldUseColor
// ---------------------------------------------------------------------------

func TestShouldUseColor_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CLICOLOR_FORCE", "")

	if shouldUseColor(&bytes.Buffer{}) {
		t.Error("shouldUseColor should return false when NO_COLOR is set")
	}
}

func TestShouldUseColor_CLIColorDisabled(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "0")
	t.Setenv("CLICOLOR_FORCE", "")

	if shouldUseColor(&bytes.Buffer{}) {
		t.Error("shouldUseColor should return false when CLICOLOR=0")
	}
}

func TestShouldUseColor_CLIColorForce(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	if !shouldUseColor(&bytes.Buffer{}) {
		t.Error("shouldUseColor should return true when CLICOLOR_FORCE is set")
	}
}

func TestShouldUseColor_NonFdWriter(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")

	// bytes.Buffer は Fd() を持たないので false になる
	if shouldUseColor(&bytes.Buffer{}) {
		t.Error("shouldUseColor should return false for non-fd writer")
	}
}

// ---------------------------------------------------------------------------

func TestBuildComposePlan_UpNoServicesDefinedAndNoFileChanges(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := remote.NewMockRemoteClient(ctrl)

	// defined services がない場合は recreate せず ComposeNoChange になる
	client.EXPECT().
		RunCommand("/srv/app", "docker compose config --services 2>/dev/null").
		Return("", nil)
	client.EXPECT().
		RunCommand("/srv/app", "docker compose ps --services --filter status=running 2>/dev/null").
		Return("", nil)

	plan := buildComposePlan(config.ComposeActionUp, "/srv/app", client, true)
	if plan == nil {
		t.Fatal("plan should not be nil")
	}

	// defined が 0 なので hasFileChanges=true でも ComposeNoChange
	if plan.ActionType != ComposeNoChange {
		t.Errorf("ActionType = %v, want ComposeNoChange when no services defined", plan.ActionType)
	}
}
