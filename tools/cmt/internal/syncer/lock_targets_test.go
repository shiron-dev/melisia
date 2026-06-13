package syncer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

// stubResolver satisfies config.SSHConfigResolver without invoking ssh.
type stubResolver struct{}

func (stubResolver) Resolve(entry *config.HostEntry, _, _ string) error {
	if entry.Port == 0 {
		entry.Port = 22
	}

	return nil
}

func writeLockTargetRepo(t *testing.T, projects ...string) *config.CmtConfig {
	t.Helper()

	base := t.TempDir()

	for _, p := range projects {
		err := os.MkdirAll(filepath.Join(base, "projects", p), 0o750)
		if err != nil {
			t.Fatalf("unexpected error creating project %q: %v", p, err)
		}
	}

	return &config.CmtConfig{
		BasePath: base,
		Defaults: &config.SyncDefaults{RemotePath: "/opt/compose"},
		Hosts: []config.HostEntry{
			{Name: "host1", Host: "host1-alias", User: "deploy"},
		},
	}
}

func TestResolveLockTargets_AllProjects(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana", "n8n")

	targets, err := ResolveLockTargets(cfg, nil, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("targets = %d, want 2", len(targets))
	}

	byProject := map[string]string{}
	for _, target := range targets {
		byProject[target.Project] = target.LockPath
	}

	for _, p := range []string{"grafana", "n8n"} {
		want := "/opt/compose/" + p + "/.cmt.lock"
		if byProject[p] != want {
			t.Errorf("lock path for %q = %q, want %q", p, byProject[p], want)
		}
	}
}

func TestResolveLockTargets_ProjectFilter(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana", "n8n")

	targets, err := ResolveLockTargets(cfg, nil, []string{"n8n"}, PlanDependencies{SSHResolver: stubResolver{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}

	if targets[0].Project != "n8n" {
		t.Errorf("project = %q, want n8n", targets[0].Project)
	}

	if targets[0].LockPath != "/opt/compose/n8n/.cmt.lock" {
		t.Errorf("lock path = %q, want /opt/compose/n8n/.cmt.lock", targets[0].LockPath)
	}
}

func TestResolveLockTargets_RemotePathOverride(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")

	// Per-host host.yml overriding remotePath to /var.
	hostDir := filepath.Join(cfg.BasePath, "hosts", "host1")

	err := os.MkdirAll(hostDir, 0o750)
	if err != nil {
		t.Fatalf("unexpected error creating host dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte("remotePath: /var\n"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing host.yml: %v", err)
	}

	targets, err := ResolveLockTargets(cfg, nil, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(targets))
	}

	if targets[0].LockPath != "/var/grafana/.cmt.lock" {
		t.Errorf("lock path = %q, want /var/grafana/.cmt.lock", targets[0].LockPath)
	}
}

func TestResolveLockTargets_NoProjects(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")

	_, err := ResolveLockTargets(cfg, nil, []string{"nonexistent"}, PlanDependencies{SSHResolver: stubResolver{}})
	if !errors.Is(err, errNoProjectsFound) {
		t.Errorf("expected errNoProjectsFound, got %v", err)
	}
}

func TestResolveLockTargets_HostConfigError(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")

	hostDir := filepath.Join(cfg.BasePath, "hosts", "host1")

	err := os.MkdirAll(hostDir, 0o750)
	if err != nil {
		t.Fatalf("unexpected error creating host dir: %v", err)
	}

	// Invalid YAML triggers a host config load error.
	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte("remotePath: [unterminated\n"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing host.yml: %v", err)
	}

	_, err = ResolveLockTargets(cfg, nil, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if err == nil {
		t.Fatal("expected error for invalid host.yml")
	}
}

func TestResolveLockTargets_NoHostProjectsMatched(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")

	hostDir := filepath.Join(cfg.BasePath, "hosts", "host1")

	err := os.MkdirAll(hostDir, 0o750)
	if err != nil {
		t.Fatalf("unexpected error creating host dir: %v", err)
	}

	// host.yml declares only an unrelated project, so grafana matches no host project.
	err = os.WriteFile(filepath.Join(hostDir, "host.yml"), []byte("projects:\n  other: {}\n"), 0o600)
	if err != nil {
		t.Fatalf("unexpected error writing host.yml: %v", err)
	}

	_, err = ResolveLockTargets(cfg, nil, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if !errors.Is(err, errNoHostProjectsMatched) {
		t.Errorf("expected errNoHostProjectsMatched, got %v", err)
	}
}

func TestResolveLockTargets_NoHostsMatched(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")

	_, err := ResolveLockTargets(cfg, []string{"nonexistent-host"}, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if !errors.Is(err, errNoHostsMatched) {
		t.Errorf("expected errNoHostsMatched, got %v", err)
	}
}

func TestResolveLockTargets_RemotePathNotSet(t *testing.T) {
	t.Parallel()

	cfg := writeLockTargetRepo(t, "grafana")
	cfg.Defaults = nil

	_, err := ResolveLockTargets(cfg, nil, nil, PlanDependencies{SSHResolver: stubResolver{}})
	if !errors.Is(err, errRemotePathNotSet) {
		t.Errorf("expected errRemotePathNotSet, got %v", err)
	}
}
