package config

import (
	"errors"
	"path/filepath"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestParseSSHGOutput(t *testing.T) {
	t.Parallel()

	assertSingle := func(t *testing.T, single map[string]string, key, want string) {
		t.Helper()

		if single[key] != want {
			t.Errorf("%s = %q, want %q", key, single[key], want)
		}
	}

	assertMulti := func(t *testing.T, multi map[string][]string, key string, want []string) {
		t.Helper()

		got := multi[key]
		if len(got) != len(want) {
			t.Fatalf("%s length = %d, want %d", key, len(got), len(want))
		}

		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s[%d] = %q, want %q", key, i, got[i], want[i])
			}
		}
	}

	testCases := []struct {
		name     string
		input    string
		validate func(*testing.T, map[string]string, map[string][]string)
	}{
		{
			name:  "basic key-value pairs",
			input: "hostname 192.168.1.1\nuser deploy\nport 2222\n",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "192.168.1.1")
				assertSingle(t, single, "user", "deploy")
				assertSingle(t, single, "port", "2222")
				assertMulti(t, multi, "hostname", []string{"192.168.1.1"})
			},
		},
		{
			name:  "keys are lowercased",
			input: "HostName example.com\nUser admin\nPort 22\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "example.com")
				assertSingle(t, single, "user", "admin")
				assertSingle(t, single, "port", "22")
			},
		},
		{
			name:  "multi-value keys",
			input: "identityfile ~/.ssh/id_rsa\nidentityfile ~/.ssh/id_ed25519\nhostname host1\n",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()
				assertSingle(t, single, "identityfile", "~/.ssh/id_ed25519")
				assertMulti(t, multi, "identityfile", []string{"~/.ssh/id_rsa", "~/.ssh/id_ed25519"})
			},
		},
		{
			name:  "invalid lines are skipped",
			input: "hostname example.com\nno-space-line\n\nport 22\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "hostname", "example.com")
				assertSingle(t, single, "port", "22")

				if _, ok := single["no-space-line"]; ok {
					t.Error("no-space-line should be skipped")
				}
			},
		},
		{
			name:  "empty input",
			input: "",
			validate: func(t *testing.T, single map[string]string, multi map[string][]string) {
				t.Helper()

				if len(single) != 0 {
					t.Errorf("expected empty single, got %v", single)
				}

				if len(multi) != 0 {
					t.Errorf("expected empty multi, got %v", multi)
				}
			},
		},
		{
			name:  "value with spaces",
			input: "proxycommand ssh -W %h:%p bastion\n",
			validate: func(t *testing.T, single map[string]string, _ map[string][]string) {
				t.Helper()
				assertSingle(t, single, "proxycommand", "ssh -W %h:%p bastion")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			single, multi := parseSSHGOutput(tc.input)
			tc.validate(t, single, multi)
		})
	}
}

// ---------------------------------------------------------------------------
// expandProxyPlaceholders
// ---------------------------------------------------------------------------

func TestExpandProxyPlaceholders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		template string
		host     string
		port     int
		user     string
		origName string
		want     string
	}{
		{
			name:     "all placeholders",
			template: "ssh -W %h:%p -l %r %n",
			host:     "192.168.1.1",
			port:     2222,
			user:     "deploy",
			origName: "myhost",
			want:     "ssh -W 192.168.1.1:2222 -l deploy myhost",
		},
		{
			name:     "percent-percent escaping",
			template: "echo %%h is %h",
			host:     "resolved.host",
			port:     22,
			user:     "user",
			origName: "orig",
			want:     "echo %h is resolved.host",
		},
		{
			name:     "no placeholders",
			template: "nc bastion 22",
			host:     "host",
			port:     22,
			user:     "user",
			origName: "orig",
			want:     "nc bastion 22",
		},
		{
			name:     "multiple percent-percent",
			template: "a%%b%%c",
			host:     "h",
			port:     1,
			user:     "u",
			origName: "o",
			want:     "a%b%c",
		},
		{
			name:     "repeated placeholders",
			template: "%h-%h-%p-%p",
			host:     "host",
			port:     80,
			user:     "user",
			origName: "orig",
			want:     "host-host-80-80",
		},
		{
			name:     "identity file path expansion",
			template: "/home/%r/.ssh/id_%n",
			host:     "10.0.0.1",
			port:     22,
			user:     "admin",
			origName: "myserver",
			want:     "/home/admin/.ssh/id_myserver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := expandProxyPlaceholders(tt.template, tt.host, tt.port, tt.user, tt.origName)
			if result != tt.want {
				t.Errorf("expandProxyPlaceholders() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestResolveSSHConfigWithRunner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	runner := NewMockSSHConfigRunner(ctrl)

	entry := &HostEntry{
		Name: "server1",
		Host: "server1-alias",
		User: "",
		Port: 0,
	}

	hostDir := "/tmp/base/hosts/server1"
	sshConfigPath := "ssh_config"

	resolved := "" +
		"hostname 10.0.0.10\n" +
		"user deploy\n" +
		"port 2222\n" +
		"proxycommand ssh -W %h:%p bastion\n" +
		"identityfile /home/%r/.ssh/id_%n\n" +
		"identityagent /tmp/agent.sock\n"

	runner.EXPECT().
		SSHOutput(
			"-G",
			"-F",
			filepath.Join(hostDir, sshConfigPath),
			"server1-alias",
		).
		Return([]byte(resolved), nil)

	err := ResolveSSHConfigWithRunner(entry, sshConfigPath, hostDir, runner)
	if err != nil {
		t.Fatalf("ResolveSSHConfigWithRunner: %v", err)
	}

	if entry.Host != "10.0.0.10" {
		t.Errorf("Host = %q, want 10.0.0.10", entry.Host)
	}

	if entry.User != "deploy" {
		t.Errorf("User = %q, want deploy", entry.User)
	}

	if entry.Port != 2222 {
		t.Errorf("Port = %d, want 2222", entry.Port)
	}

	if entry.ProxyCommand != "ssh -W 10.0.0.10:2222 bastion" {
		t.Errorf("ProxyCommand = %q", entry.ProxyCommand)
	}

	if len(entry.IdentityFiles) != 1 || entry.IdentityFiles[0] != "/home/deploy/.ssh/id_server1-alias" {
		t.Errorf("IdentityFiles = %v", entry.IdentityFiles)
	}

	if entry.IdentityAgent != "/tmp/agent.sock" {
		t.Errorf("IdentityAgent = %q", entry.IdentityAgent)
	}
}

func TestResolveSSHConfigWithRunner_SSHError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	runner := NewMockSSHConfigRunner(ctrl)

	entry := &HostEntry{Name: "server1", Host: "server1-alias"}

	runner.EXPECT().
		SSHOutput(gomock.Any()).
		Return(nil, errors.New("connection refused"))

	err := ResolveSSHConfigWithRunner(entry, "", "/tmp/hosts/server1", runner)
	if err == nil {
		t.Fatal("expected error when SSH fails")
	}
}

func TestResolveSSHConfigWithRunner_NoSshConfig(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	runner := NewMockSSHConfigRunner(ctrl)

	entry := &HostEntry{Name: "server1", Host: "server1-alias", User: "deploy", Port: 22}

	// sshConfigPath が空の場合、-F オプションなしで ssh -G が呼ばれます。
	runner.EXPECT().
		SSHOutput("-G", "-l", "deploy", "-p", "22", "server1-alias").
		Return([]byte("hostname 10.0.0.1\nport 22\nuser deploy\n"), nil)

	err := ResolveSSHConfigWithRunner(entry, "", "/tmp/hosts/server1", runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if entry.Host != "10.0.0.1" {
		t.Errorf("Host = %q, want 10.0.0.1", entry.Host)
	}
}

func TestBuildSSHGArgs_AbsoluteSshConfigPath(t *testing.T) {
	t.Parallel()

	entry := &HostEntry{Host: "server1"}
	// 絶対パスの sshConfigPath は Join されず、そのまま使われます。
	args := buildSSHGArgs(entry, "/etc/ssh/custom_config", "/tmp/base/hosts/server1")

	if !containsSSHSeq(args, "-F", "/etc/ssh/custom_config") {
		t.Errorf("args should contain -F /etc/ssh/custom_config, got %v", args)
	}
}

func TestBuildSSHGArgs_EmptySshConfigPath(t *testing.T) {
	t.Parallel()

	entry := &HostEntry{Host: "server1", User: "deploy"}
	args := buildSSHGArgs(entry, "", "/tmp/base/hosts/server1")

	for _, arg := range args {
		if arg == "-F" {
			t.Errorf("args should not contain -F when sshConfigPath is empty, got %v", args)
		}
	}

	if !containsSSHSeq(args, "-l", "deploy") {
		t.Errorf("args should contain -l deploy, got %v", args)
	}
}

func TestBuildSSHGArgs_WithPort(t *testing.T) {
	t.Parallel()

	entry := &HostEntry{Host: "server1", Port: 2222}
	args := buildSSHGArgs(entry, "", "/tmp/base/hosts/server1")

	if !containsSSHSeq(args, "-p", "2222") {
		t.Errorf("args should contain -p 2222, got %v", args)
	}
}

// containsSSHSeq は args の中に key, val の連続ペアが含まれるかを返します。
func containsSSHSeq(args []string, key, val string) bool {
	for i := range args {
		if args[i] == key && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}

	return false
}
