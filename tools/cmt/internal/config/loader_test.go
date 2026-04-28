package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadCmtConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cfgContent := `
basePath: ./compose
hosts:
  - name: server1
    host: 192.168.1.1
    user: deploy
    sshAgent: true
  - name: server2
    host: 192.168.1.2
    port: 2222
    user: deploy
    sshKeyPath: /home/deploy/.ssh/id_ed25519
defaults:
  remotePath: /opt/compose
  postSyncCommand: docker compose up -d
`

	cfgPath := filepath.Join(dir, "config.yml")

	err := os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	if err != nil {
		t.Fatal(err)
	}

	// basePath が解決できるように compose ディレクトリを作成します。
	err = os.MkdirAll(filepath.Join(dir, "compose"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCmtConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadCmtConfig: %v", err)
	}

	// basePath は絶対パスに解決されているはずです。
	if !filepath.IsAbs(cfg.BasePath) {
		t.Errorf("basePath should be absolute, got %q", cfg.BasePath)
	}

	if len(cfg.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(cfg.Hosts))
	}
	// ポートは設定レベルでは 0 がデフォルト。22 は後で ResolveSSHConfig により適用されます。
	if cfg.Hosts[0].Port != 0 {
		t.Errorf("default port should be 0 (unset), got %d", cfg.Hosts[0].Port)
	}

	if cfg.Hosts[1].Port != 2222 {
		t.Errorf("server2 port should be 2222, got %d", cfg.Hosts[1].Port)
	}

	if cfg.Defaults == nil {
		t.Fatal("defaults should not be nil")
	}

	if cfg.Defaults.RemotePath != "/opt/compose" {
		t.Errorf("remotePath = %q", cfg.Defaults.RemotePath)
	}
}

func TestLoadCmtConfig_WithBeforeApplyHooks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cfgContent := `
basePath: ./compose
hosts:
  - name: server1
    host: 192.168.1.1
    user: deploy
beforeApplyHooks:
  beforePlan:
    command: ./scripts/prepare-context.sh
  beforeApplyPrompt:
    command: ./scripts/check-policy.sh
  beforeApply:
    command: ./scripts/final-gate.sh
`

	cfgPath := filepath.Join(dir, "config.yml")

	err := os.WriteFile(cfgPath, []byte(cfgContent), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(dir, "compose"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadCmtConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadCmtConfig: %v", err)
	}

	if cfg.BeforeApplyHooks == nil {
		t.Fatal("beforeApplyHooks should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforePlan == nil {
		t.Fatal("beforePlan should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforePlan.Command != "./scripts/prepare-context.sh" {
		t.Errorf("beforePlan.command = %q", cfg.BeforeApplyHooks.BeforePlan.Command)
	}

	if cfg.BeforeApplyHooks.BeforeApplyPrompt == nil {
		t.Fatal("beforeApplyPrompt should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforeApplyPrompt.Command != "./scripts/check-policy.sh" {
		t.Errorf("beforeApplyPrompt.command = %q", cfg.BeforeApplyHooks.BeforeApplyPrompt.Command)
	}

	if cfg.BeforeApplyHooks.BeforeApply == nil {
		t.Fatal("beforeApply should not be nil")
	}

	if cfg.BeforeApplyHooks.BeforeApply.Command != "./scripts/final-gate.sh" {
		t.Errorf("beforeApply.command = %q", cfg.BeforeApplyHooks.BeforeApply.Command)
	}
}

func TestLoadCmtConfig_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "missing basePath",
			content: "hosts:\n  - name: x\n    host: x\n    user: x\n",
			wantErr: true,
		},
		{
			name:    "no hosts",
			content: "basePath: .\nhosts: []\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "bad.yml")

			err := os.WriteFile(cfgPath, []byte(tt.content), 0600)
			if err != nil {
				t.Fatal(err)
			}

			_, err = LoadCmtConfig(cfgPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadCmtConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoverProjects(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	projDir := filepath.Join(dir, "projects")

	err := os.MkdirAll(filepath.Join(projDir, "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.MkdirAll(filepath.Join(projDir, "prometheus"), 0750)
	if err != nil {
		t.Fatal(err)
	}
	// 通常ファイルは無視されるべきです。
	err = os.WriteFile(filepath.Join(projDir, "README.md"), []byte("hi"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := DiscoverProjects(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(projects), projects)
	}
}

func TestFilterHosts(t *testing.T) {
	t.Parallel()

	hosts := []HostEntry{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	}

	tests := []struct {
		name      string
		filter    []string
		wantCount int
		wantNames []string
	}{
		{
			name:      "nil filter returns all",
			filter:    nil,
			wantCount: 3,
			wantNames: []string{"a", "b", "c"},
		},
		{
			name:      "specific filter",
			filter:    []string{"a", "c"},
			wantCount: 2,
			wantNames: []string{"a", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FilterHosts(hosts, tt.filter)
			if len(got) != tt.wantCount {
				t.Errorf("FilterHosts() count = %d, want %d", len(got), tt.wantCount)
			}

			for i, name := range tt.wantNames {
				if i >= len(got) || got[i].Name != name {
					t.Errorf("FilterHosts() hosts = %v, want names %v", got, tt.wantNames)

					break
				}
			}
		})
	}
}

func TestFilterProjects(t *testing.T) {
	t.Parallel()

	projects := []string{"grafana", "prometheus", "loki"}

	tests := []struct {
		name   string
		filter []string
		want   []string
	}{
		{
			name:   "nil filter returns all",
			filter: nil,
			want:   []string{"grafana", "prometheus", "loki"},
		},
		{
			name:   "specific filter",
			filter: []string{"grafana"},
			want:   []string{"grafana"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FilterProjects(projects, tt.filter)
			if len(got) != len(tt.want) {
				t.Errorf("FilterProjects() = %v, want %v", got, tt.want)

				return
			}

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("FilterProjects() = %v, want %v", got, tt.want)

					break
				}
			}
		})
	}
}

func TestResolveProjectConfig(t *testing.T) {
	t.Parallel()

	cmtDefaults := &SyncDefaults{
		RemotePath:      "/opt/default",
		PostSyncCommand: "echo default",
	}
	hostCfg := &HostConfig{
		RemotePath:      "/opt/host",
		PostSyncCommand: "",
		Projects: map[string]*ProjectConfig{
			"grafana": {
				PostSyncCommand: "docker compose up -d",
			},
		},
	}

	tests := []struct {
		name            string
		hostCfg         *HostConfig
		project         string
		wantRemotePath  string
		wantPostCommand string
	}{
		{
			name:            "layer 1 only",
			hostCfg:         nil,
			project:         "grafana",
			wantRemotePath:  "/opt/default",
			wantPostCommand: "echo default",
		},
		{
			name:            "layer 2 overrides path, layer 1 provides command",
			hostCfg:         hostCfg,
			project:         "prometheus",
			wantRemotePath:  "/opt/host",
			wantPostCommand: "echo default",
		},
		{
			name:            "layer 3 overrides command",
			hostCfg:         hostCfg,
			project:         "grafana",
			wantRemotePath:  "/opt/host",
			wantPostCommand: "docker compose up -d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(cmtDefaults, tt.hostCfg, tt.project)
			if resolved.RemotePath != tt.wantRemotePath {
				t.Errorf("RemotePath = %q, want %q", resolved.RemotePath, tt.wantRemotePath)
			}

			if resolved.PostSyncCommand != tt.wantPostCommand {
				t.Errorf("PostSyncCommand = %q, want %q", resolved.PostSyncCommand, tt.wantPostCommand)
			}
		})
	}
}

func TestResolveProjectConfig_ComposeAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		defaults   *SyncDefaults
		hostCfg    *HostConfig
		project    string
		wantAction string
	}{
		{
			name:       "defaults to up when unset",
			defaults:   &SyncDefaults{RemotePath: "/opt"},
			hostCfg:    nil,
			project:    "grafana",
			wantAction: ComposeActionUp,
		},
		{
			name:       "defaults level sets action",
			defaults:   &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionDown},
			hostCfg:    nil,
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "host level overrides defaults",
			defaults: &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionUp},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionDown,
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "project level overrides host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionUp,
				Projects: map[string]*ProjectConfig{
					"grafana": {ComposeAction: ComposeActionDown},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "unset project inherits host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				ComposeAction: ComposeActionDown,
				Projects: map[string]*ProjectConfig{
					"grafana": {},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionDown,
		},
		{
			name:     "project can ignore compose runtime state",
			defaults: &SyncDefaults{RemotePath: "/opt", ComposeAction: ComposeActionUp},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"grafana": {ComposeAction: ComposeActionIgnore},
				},
			},
			project:    "grafana",
			wantAction: ComposeActionIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(tt.defaults, tt.hostCfg, tt.project)
			if resolved.ComposeAction != tt.wantAction {
				t.Errorf("ComposeAction = %q, want %q", resolved.ComposeAction, tt.wantAction)
			}
		})
	}
}

func TestResolveProjectConfig_TemplateVarSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		defaults    *SyncDefaults
		hostCfg     *HostConfig
		project     string
		wantSources []string
	}{
		{
			name:        "defaults to DefaultTemplateVarSources when unset",
			defaults:    &SyncDefaults{RemotePath: "/opt"},
			hostCfg:     nil,
			project:     "grafana",
			wantSources: DefaultTemplateVarSources(),
		},
		{
			name:        "defaults level sets sources",
			defaults:    &SyncDefaults{RemotePath: "/opt", TemplateVarSources: []string{"*.toml"}},
			hostCfg:     nil,
			project:     "grafana",
			wantSources: []string{"*.toml"},
		},
		{
			name:     "host level overrides defaults",
			defaults: &SyncDefaults{RemotePath: "/opt", TemplateVarSources: []string{"*.toml"}},
			hostCfg: &HostConfig{
				TemplateVarSources: []string{"env.yml"},
			},
			project:     "grafana",
			wantSources: []string{"env.yml"},
		},
		{
			name:     "project level overrides host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				TemplateVarSources: []string{"env.yml"},
				Projects: map[string]*ProjectConfig{
					"grafana": {TemplateVarSources: []string{"secrets.yaml"}},
				},
			},
			project:     "grafana",
			wantSources: []string{"secrets.yaml"},
		},
		{
			name:     "unset project inherits host",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				TemplateVarSources: []string{"env.yml"},
				Projects: map[string]*ProjectConfig{
					"grafana": {},
				},
			},
			project:     "grafana",
			wantSources: []string{"env.yml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(tt.defaults, tt.hostCfg, tt.project)
			if len(resolved.TemplateVarSources) != len(tt.wantSources) {
				t.Fatalf("TemplateVarSources = %v, want %v", resolved.TemplateVarSources, tt.wantSources)
			}

			for i, s := range tt.wantSources {
				if resolved.TemplateVarSources[i] != s {
					t.Errorf("TemplateVarSources[%d] = %q, want %q", i, resolved.TemplateVarSources[i], s)
				}
			}
		})
	}
}

func TestResolveProjectConfig_RemoveOrphans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		defaults          *SyncDefaults
		hostCfg           *HostConfig
		project           string
		wantRemoveOrphans bool
	}{
		{
			name:              "defaults to false when unset",
			defaults:          &SyncDefaults{RemotePath: "/opt"},
			hostCfg:           nil,
			project:           "grafana",
			wantRemoveOrphans: false,
		},
		{
			name:     "project level enables remove-orphans",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"grafana": {RemoveOrphans: true},
				},
			},
			project:           "grafana",
			wantRemoveOrphans: true,
		},
		{
			name:     "missing project keeps false",
			defaults: &SyncDefaults{RemotePath: "/opt"},
			hostCfg: &HostConfig{
				Projects: map[string]*ProjectConfig{
					"prometheus": {RemoveOrphans: true},
				},
			},
			project:           "grafana",
			wantRemoveOrphans: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			resolved := ResolveProjectConfig(tt.defaults, tt.hostCfg, tt.project)
			if resolved.RemoveOrphans != tt.wantRemoveOrphans {
				t.Errorf("RemoveOrphans = %v, want %v", resolved.RemoveOrphans, tt.wantRemoveOrphans)
			}
		})
	}
}

func TestLoadHostConfig_ComposeAction(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
remotePath: /srv/compose
composeAction: down
projects:
  grafana:
    composeAction: up
    removeOrphans: true
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	if hostConfig.ComposeAction != "down" {
		t.Errorf("host composeAction = %q, want %q", hostConfig.ComposeAction, "down")
	}

	if hostConfig.Projects["grafana"].ComposeAction != "up" {
		t.Errorf("grafana composeAction = %q, want %q", hostConfig.Projects["grafana"].ComposeAction, "up")
	}

	if !hostConfig.Projects["grafana"].RemoveOrphans {
		t.Error("grafana removeOrphans should be true")
	}
}

func TestLoadHostConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// host.yml が無い場合 → nil, nil。
	hostConfig, err := LoadHostConfig(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected not-found error for missing host.yml")
	}

	if hostConfig != nil {
		t.Error("expected nil for missing host.yml")
	}

	// 有効な host.yml。
	hostDir := filepath.Join(dir, "hosts", "server1")

	err = os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
remotePath: /srv/compose
postSyncCommand: docker compose up -d
projects:
  grafana:
    postSyncCommand: docker compose -f compose.yml -f compose.override.yml up -d
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err = LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	if hostConfig.RemotePath != "/srv/compose" {
		t.Errorf("remotePath = %q", hostConfig.RemotePath)
	}

	if hostConfig.Projects["grafana"] == nil {
		t.Fatal("grafana project config missing")
	}
}

func TestLoadHostConfig_DirsStringFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  grafana:
    dirs:
      - grafana_storage
      - grafana_conf
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	dirs := hostConfig.Projects["grafana"].Dirs
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}

	if dirs[0].Path != "grafana_storage" {
		t.Errorf("dirs[0].Path = %q, want %q", dirs[0].Path, "grafana_storage")
	}

	if dirs[1].Path != "grafana_conf" {
		t.Errorf("dirs[1].Path = %q, want %q", dirs[1].Path, "grafana_conf")
	}

	if dirs[0].Permission != "" || dirs[0].Owner != "" || dirs[0].Group != "" {
		t.Error("string-format dir should have empty permission/owner/group")
	}
}

func TestLoadHostConfig_DirsRecursive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  snipeit:
    dirs:
      - redis_data:
          owner: 1000
          group: 1000
          recursive: true
          become: true
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	dirs := hostConfig.Projects["snipeit"].Dirs
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}

	if dirs[0].Path != "redis_data" {
		t.Errorf("Path = %q, want %q", dirs[0].Path, "redis_data")
	}

	if dirs[0].Owner != "1000" || dirs[0].Group != "1000" {
		t.Errorf("Owner = %q, Group = %q", dirs[0].Owner, dirs[0].Group)
	}

	if !dirs[0].Recursive {
		t.Error("Recursive = false, want true")
	}
}

func TestLoadHostConfig_DirsPathKeyedFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  grafana:
    dirs:
      - influxdb_data:
        permission: "0755"
        owner: influxdb
        group: influxdb
        become: true
        becomeUser: root
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	dirs := hostConfig.Projects["grafana"].Dirs
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}

	if dirs[0].Path != "influxdb_data" {
		t.Errorf("Path = %q", dirs[0].Path)
	}

	if dirs[0].Permission != "0755" {
		t.Errorf("Permission = %q, want %q", dirs[0].Permission, "0755")
	}

	if dirs[0].Owner != "influxdb" {
		t.Errorf("Owner = %q, want %q", dirs[0].Owner, "influxdb")
	}

	if dirs[0].Group != "influxdb" {
		t.Errorf("Group = %q, want %q", dirs[0].Group, "influxdb")
	}

	if !dirs[0].Become {
		t.Error("Become should be true")
	}

	if dirs[0].BecomeUser != "root" {
		t.Errorf("BecomeUser = %q, want %q", dirs[0].BecomeUser, "root")
	}
}

func TestLoadHostConfig_DirsMixedFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  grafana:
    dirs:
      - simple_dir
      - managed_dir:
        permission: "0750"
        owner: app
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	hostConfig, err := LoadHostConfig(dir, "server1")
	if err != nil {
		t.Fatal(err)
	}

	dirs := hostConfig.Projects["grafana"].Dirs
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}

	if dirs[0].Path != "simple_dir" {
		t.Errorf("dirs[0].Path = %q", dirs[0].Path)
	}

	if dirs[0].Permission != "" {
		t.Errorf("dirs[0].Permission should be empty, got %q", dirs[0].Permission)
	}

	if dirs[1].Path != "managed_dir" {
		t.Errorf("dirs[1].Path = %q", dirs[1].Path)
	}

	if dirs[1].Permission != "0750" {
		t.Errorf("dirs[1].Permission = %q", dirs[1].Permission)
	}

	if dirs[1].Owner != "app" {
		t.Errorf("dirs[1].Owner = %q", dirs[1].Owner)
	}
}

func TestLoadHostConfig_DirsPathObjectFormat_IsRejected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  grafana:
    dirs:
      - path: legacy_dir
        owner: legacy
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadHostConfig(dir, "server1")
	if err == nil {
		t.Fatal("expected error for unsupported dirs path object format")
	}

	if !strings.Contains(err.Error(), "invalid dirs item") {
		t.Errorf("error should mention invalid dirs item, got %q", err.Error())
	}
}

func TestLoadHostConfig_DirsInvalidPermission(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	hostDir := filepath.Join(dir, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	content := `
projects:
  grafana:
    dirs:
      - data:
        permission: "not-octal"
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadHostConfig(dir, "server1")
	if err == nil {
		t.Fatal("expected error for invalid permission")
	}

	if !strings.Contains(err.Error(), "invalid permission") {
		t.Errorf("error should mention invalid permission, got %q", err.Error())
	}
}

func TestValidateDirConfigs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dirs    []DirConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid string-style dirs",
			dirs:    []DirConfig{{Path: "data"}, {Path: "logs"}},
			wantErr: false,
		},
		{
			name:    "valid with permission",
			dirs:    []DirConfig{{Path: "data", Permission: "0755"}},
			wantErr: false,
		},
		{
			name:    "empty path",
			dirs:    []DirConfig{{Path: ""}},
			wantErr: true,
			errMsg:  "path is required",
		},
		{
			name:    "invalid permission",
			dirs:    []DirConfig{{Path: "data", Permission: "abc"}},
			wantErr: true,
			errMsg:  "invalid permission",
		},
		{
			name:    "nil dirs",
			dirs:    nil,
			wantErr: false,
		},
		{
			name:    "become user without become",
			dirs:    []DirConfig{{Path: "data", BecomeUser: "root"}},
			wantErr: true,
			errMsg:  "becomeUser requires become=true",
		},
		{
			name:    "valid with recursive",
			dirs:    []DirConfig{{Path: "redis_data", Owner: "1000", Group: "1000", Recursive: true}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateDirConfigs(tt.dirs)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateDirConfigs() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestResolveProjectConfig_Dirs(t *testing.T) {
	t.Parallel()

	cmtDefaults := &SyncDefaults{RemotePath: "/opt"}
	hostCfg := &HostConfig{
		Projects: map[string]*ProjectConfig{
			"grafana": {
				Dirs: []DirConfig{
					{Path: "data"},
					{Path: "logs", Permission: "0755", Owner: "app", Group: "app"},
				},
			},
		},
	}

	resolved := ResolveProjectConfig(cmtDefaults, hostCfg, "grafana")

	if len(resolved.Dirs) != 2 {
		t.Fatalf("Dirs = %d, want 2", len(resolved.Dirs))
	}

	if resolved.Dirs[0].Path != "data" {
		t.Errorf("Dirs[0].Path = %q", resolved.Dirs[0].Path)
	}

	if resolved.Dirs[1].Permission != "0755" {
		t.Errorf("Dirs[1].Permission = %q", resolved.Dirs[1].Permission)
	}

	if resolved.Dirs[1].Owner != "app" {
		t.Errorf("Dirs[1].Owner = %q", resolved.Dirs[1].Owner)
	}
}

// ---------------------------------------------------------------------------
// resolveFromDefaults - nil branch
// ---------------------------------------------------------------------------

func TestResolveProjectConfig_NilDefaults(t *testing.T) {
	t.Parallel()

	// defaults が nil のとき、空の ResolvedProjectConfig が返ることを確認します。
	resolved := ResolveProjectConfig(nil, nil, "grafana")

	if resolved.RemotePath != "" {
		t.Errorf("RemotePath = %q, want empty", resolved.RemotePath)
	}

	if resolved.PostSyncCommand != "" {
		t.Errorf("PostSyncCommand = %q, want empty", resolved.PostSyncCommand)
	}

	if resolved.ComposeAction != ComposeActionUp {
		t.Errorf("ComposeAction = %q, want %q (default)", resolved.ComposeAction, ComposeActionUp)
	}

	if resolved.RemoveOrphans {
		t.Error("RemoveOrphans should be false")
	}
}

// ---------------------------------------------------------------------------
// DirConfig.UnmarshalYAML - SequenceNode (defensive branch)
// ---------------------------------------------------------------------------

func TestDirConfig_UnmarshalYAML_SequenceNode(t *testing.T) {
	t.Parallel()

	// sequence型のYAMLをDirConfigにUnmarshalしようとするとエラーになることを確認します。
	type wrapper struct {
		Dir DirConfig `yaml:"dir"`
	}

	var w wrapper

	yamlInput := "dir:\n  - item1\n  - item2\n"

	err := unmarshalYAML([]byte(yamlInput), &w)
	if err == nil {
		t.Error("expected error when unmarshaling sequence node into DirConfig")
	}
}

// ---------------------------------------------------------------------------
// mergeNonEmptyDirConfigAttrs - BecomeUser via nested mapping form
// ---------------------------------------------------------------------------

func TestLoadHostConfig_Dirs_BecomeUser_NestedMapping(t *testing.T) {
	t.Parallel()

	// path-key形式でネストされたmapping値の中に becomeUser を含む場合のテストです。
	// この形式は mergeNonEmptyDirConfigAttrs の BecomeUser 分岐をカバーします。
	base := t.TempDir()

	hostDir := filepath.Join(base, "hosts", "server1")

	err := os.MkdirAll(hostDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	// path-key形式: パスをキーとし、attrs をネストされたmappingで指定します。
	hostYAML := `
projects:
  app:
    dirs:
      - /srv/data:
          permission: "0750"
          owner: app
          group: app
          become: true
          becomeUser: deploy
`

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte(hostYAML), 0600)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadHostConfig(base, "server1")
	if err != nil {
		t.Fatalf("LoadHostConfig: %v", err)
	}

	proj := cfg.Projects["app"]
	if proj == nil {
		t.Fatal("project 'app' not found")
	}

	if len(proj.Dirs) != 1 {
		t.Fatalf("Dirs = %d, want 1", len(proj.Dirs))
	}

	d := proj.Dirs[0]

	if d.Path != "/srv/data" {
		t.Errorf("Path = %q, want %q", d.Path, "/srv/data")
	}

	if !d.Become {
		t.Error("Become should be true")
	}

	if d.BecomeUser != "deploy" {
		t.Errorf("BecomeUser = %q, want %q", d.BecomeUser, "deploy")
	}
}

// ---------------------------------------------------------------------------
// applyHostOverrides - TemplateVarSources branch
// ---------------------------------------------------------------------------

func TestResolveProjectConfig_HostTemplateVarSources(t *testing.T) {
	t.Parallel()

	defaults := &SyncDefaults{RemotePath: "/opt"}
	hostCfg := &HostConfig{
		TemplateVarSources: []string{"*.env", "secrets.yml"},
	}

	resolved := ResolveProjectConfig(defaults, hostCfg, "grafana")

	if len(resolved.TemplateVarSources) != 2 {
		t.Fatalf("TemplateVarSources = %v, want 2 sources", resolved.TemplateVarSources)
	}

	if resolved.TemplateVarSources[0] != "*.env" {
		t.Errorf("TemplateVarSources[0] = %q, want %q", resolved.TemplateVarSources[0], "*.env")
	}
}

// ---------------------------------------------------------------------------
// helper for YAML unmarshaling in tests
// ---------------------------------------------------------------------------

func unmarshalYAML(data []byte, dst any) error {
	return yaml.Unmarshal(data, dst)
}
