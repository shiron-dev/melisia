package remote

import (
	"errors"
	"strings"
	"testing"

	"cmt/internal/config"
)

var (
	errExitStatus1 = errors.New("exit status 1")
	errSSHFailed   = errors.New("ssh failed")
	errExit1       = errors.New("exit 1")
)

// mockCommandRunner は CommandRunner のテスト用実装です。
type mockCommandRunner struct {
	sshOutput []byte
	sshErr    error
	scpErr    error
	// 受け取ったargs を記録します。
	capturedSSHArgs []string
	capturedSCPArgs []string
}

func (m *mockCommandRunner) SSHCombinedOutput(args ...string) ([]byte, error) {
	m.capturedSSHArgs = args

	return m.sshOutput, m.sshErr
}

func (m *mockCommandRunner) SCPCombinedOutput(args ...string) ([]byte, error) {
	m.capturedSCPArgs = args

	return nil, m.scpErr
}

// ---------------------------------------------------------------------------
// shellQuote
// ---------------------------------------------------------------------------

func TestShellQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"hello", "'hello'"},
		{"/srv/data", "'/srv/data'"},
		{"path with spaces", "'path with spaces'"},
		{"it's here", "'it'\\''s here'"},
		{"'quoted'", "''\\''quoted'\\'''"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()

			got := shellQuote(tt.input)
			if got != tt.want {
				t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// buildArgs / buildCommonOptions
// ---------------------------------------------------------------------------

func TestBuildArgs_NoPortNoUser(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{Name: "s1", Host: "h1"}
	ssh, scp := buildArgs(entry)

	// port=0 なのでポートオプションは含まない
	for _, arg := range ssh {
		if arg == "-p" {
			t.Error("ssh args should not contain -p when port is 0")
		}
	}

	for _, arg := range scp {
		if arg == "-P" {
			t.Error("scp args should not contain -P when port is 0")
		}
	}

	// user が空なのでlオプションは含まない
	for _, arg := range ssh {
		if arg == "-l" {
			t.Error("ssh args should not contain -l when user is empty")
		}
	}
}

func TestBuildArgs_StandardPort(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{Name: "s1", Host: "h1", Port: 22}
	ssh, scp := buildArgs(entry)

	for _, arg := range ssh {
		if arg == "-p" {
			t.Error("ssh args should not contain -p for standard port 22")
		}
	}

	for _, arg := range scp {
		if arg == "-P" {
			t.Error("scp args should not contain -P for standard port 22")
		}
	}
}

func TestBuildArgs_CustomPort(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{Name: "s1", Host: "h1", Port: 2222}
	ssh, scp := buildArgs(entry)

	if !containsSeq(ssh, "-p", "2222") {
		t.Errorf("ssh args should contain -p 2222, got %v", ssh)
	}

	if !containsSeq(scp, "-P", "2222") {
		t.Errorf("scp args should contain -P 2222, got %v", scp)
	}
}

func TestBuildArgs_WithUser(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{Name: "s1", Host: "h1", User: "deploy"}
	ssh, _ := buildArgs(entry)

	if !containsSeq(ssh, "-l", "deploy") {
		t.Errorf("ssh args should contain -l deploy, got %v", ssh)
	}
}

func TestBuildCommonOptions_ProxyCommand(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{ProxyCommand: "ssh -W %h:%p bastion"}
	opts := buildCommonOptions(entry)

	if !containsSeq(opts, "-o", "ProxyCommand=ssh -W %h:%p bastion") {
		t.Errorf("opts should contain ProxyCommand, got %v", opts)
	}
}

func TestBuildCommonOptions_IdentityAgent(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{IdentityAgent: "/run/ssh-agent.sock"}
	opts := buildCommonOptions(entry)

	if !containsSeq(opts, "-o", "IdentityAgent=/run/ssh-agent.sock") {
		t.Errorf("opts should contain IdentityAgent, got %v", opts)
	}
}

func TestBuildCommonOptions_IdentityAgent_NoneSkipped(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{IdentityAgent: "none"}
	opts := buildCommonOptions(entry)

	for i, arg := range opts {
		if arg == "-o" && i+1 < len(opts) && strings.HasPrefix(opts[i+1], "IdentityAgent=") {
			t.Errorf("opts should not contain IdentityAgent=none, got %v", opts)
		}
	}
}

func TestBuildCommonOptions_IdentityFiles(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{IdentityFiles: []string{"/key1", "/key2"}}
	opts := buildCommonOptions(entry)

	if !containsSeq(opts, "-i", "/key1") {
		t.Errorf("opts should contain -i /key1, got %v", opts)
	}

	if !containsSeq(opts, "-i", "/key2") {
		t.Errorf("opts should contain -i /key2, got %v", opts)
	}
}

func TestBuildCommonOptions_SSHKeyPath_FallbackWhenNoIdentityFiles(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{SSHKeyPath: "/fallback.pem"}
	opts := buildCommonOptions(entry)

	if !containsSeq(opts, "-i", "/fallback.pem") {
		t.Errorf("opts should fall back to SSHKeyPath, got %v", opts)
	}
}

func TestBuildCommonOptions_SSHKeyPath_NotUsedWhenIdentityFilesSet(t *testing.T) {
	t.Parallel()

	entry := config.HostEntry{
		SSHKeyPath:    "/fallback.pem",
		IdentityFiles: []string{"/primary.pem"},
	}
	opts := buildCommonOptions(entry)

	// /fallback.pem は IdentityFiles がある場合は使われない
	for i, arg := range opts {
		if arg == "-i" && i+1 < len(opts) && opts[i+1] == "/fallback.pem" {
			t.Errorf("opts should not use SSHKeyPath when IdentityFiles is set, got %v", opts)
		}
	}
}

// ---------------------------------------------------------------------------
// Client methods using mockCommandRunner
// ---------------------------------------------------------------------------

func TestClient_MkdirAll(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	mkdirErr := client.MkdirAll("/srv/data")
	if mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}

	cmd := strings.Join(runner.capturedSSHArgs, " ")
	if !strings.Contains(cmd, "mkdir -p '/srv/data'") {
		t.Errorf("SSH command should contain mkdir -p, got: %s", cmd)
	}
}

func TestClient_Remove(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	removeErr := client.Remove("/srv/data/old.txt")
	if removeErr != nil {
		t.Fatalf("Remove: %v", removeErr)
	}

	cmd := strings.Join(runner.capturedSSHArgs, " ")
	if !strings.Contains(cmd, "rm -f '/srv/data/old.txt'") {
		t.Errorf("SSH command should contain rm -f, got: %s", cmd)
	}
}

func TestClient_ReadFile(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("file content"), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	data, readErr := client.ReadFile("/srv/compose.yml")
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}

	if string(data) != "file content" {
		t.Errorf("ReadFile = %q, want %q", string(data), "file content")
	}

	cmd := strings.Join(runner.capturedSSHArgs, " ")
	if !strings.Contains(cmd, "cat '/srv/compose.yml'") {
		t.Errorf("SSH command should contain cat, got: %s", cmd)
	}
}

func TestClient_ReadFile_SSHError(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("ssh: connection refused"), sshErr: errExitStatus1}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, readErr := client.ReadFile("/srv/compose.yml")
	if readErr == nil {
		t.Fatal("ReadFile should return error on SSH failure")
	}
}

func TestClient_RunCommand_NoWorkdir(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("ok"), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	out, runErr := client.RunCommand("", "echo hello")
	if runErr != nil {
		t.Fatalf("RunCommand: %v", runErr)
	}

	if out != "ok" {
		t.Errorf("RunCommand = %q, want %q", out, "ok")
	}

	cmd := strings.Join(runner.capturedSSHArgs, " ")
	// workdirなしなのでそのままコマンドが渡る
	if !strings.HasSuffix(cmd, "-- echo hello") {
		t.Errorf("SSH command should end with '-- echo hello', got: %s", cmd)
	}
}

func TestClient_RunCommand_WithWorkdir(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("done"), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, runErr := client.RunCommand("/srv/grafana", "docker compose up -d")
	if runErr != nil {
		t.Fatalf("RunCommand: %v", runErr)
	}

	cmd := strings.Join(runner.capturedSSHArgs, " ")
	if !strings.Contains(cmd, "cd '/srv/grafana'") {
		t.Errorf("SSH command should contain cd, got: %s", cmd)
	}
}

func TestClient_ListFilesRecursive(t *testing.T) {
	t.Parallel()

	findOutput := "/srv/grafana/compose.yml\n/srv/grafana/config/grafana.ini\n"
	runner := &mockCommandRunner{sshOutput: []byte(findOutput), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	files, listErr := client.ListFilesRecursive("/srv/grafana")
	if listErr != nil {
		t.Fatalf("ListFilesRecursive: %v", listErr)
	}

	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2; files = %v", len(files), files)
	}

	if files[0] != "compose.yml" {
		t.Errorf("files[0] = %q, want %q", files[0], "compose.yml")
	}

	if files[1] != "config/grafana.ini" {
		t.Errorf("files[1] = %q, want %q", files[1], "config/grafana.ini")
	}
}

func TestClient_ListFilesRecursive_Empty(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	files, listErr := client.ListFilesRecursive("/srv/grafana")
	if listErr != nil {
		t.Fatalf("ListFilesRecursive: %v", listErr)
	}

	if len(files) != 0 {
		t.Errorf("expected empty file list, got %v", files)
	}
}

func TestClient_Close(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	closeErr := client.Close()
	if closeErr != nil {
		t.Errorf("Close() = %v, want nil", closeErr)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// containsSeq は args の中に key, val の連続ペアが含まれるかを返します。
func containsSeq(args []string, key, val string) bool {
	for i := range args {
		if args[i] == key && i+1 < len(args) && args[i+1] == val {
			return true
		}
	}

	return false
}

// ---------------------------------------------------------------------------
// Client.Stat
// ---------------------------------------------------------------------------

func TestClient_Stat_Exists(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	info, statErr := client.Stat("/srv/data/file.txt")
	if statErr != nil {
		t.Fatalf("Stat: %v", statErr)
	}

	if info == nil {
		t.Fatal("expected non-nil FileInfo")
	}

	if info.Name() != "file.txt" {
		t.Errorf("Name() = %q, want %q", info.Name(), "file.txt")
	}
}

func TestClient_Stat_UnknownError(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("connection refused"), sshErr: errSSHFailed}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := client.Stat("/srv/data/file.txt")
	if statErr == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(statErr, errExistenceUnknown) {
		t.Errorf("expected errExistenceUnknown, got %v", statErr)
	}
}

// ---------------------------------------------------------------------------
// Client.StatDirMetadata
// ---------------------------------------------------------------------------

func TestClient_StatDirMetadata(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("755 0 0 root root\n"), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	meta, statErr := client.StatDirMetadata("/srv/data")
	if statErr != nil {
		t.Fatalf("StatDirMetadata: %v", statErr)
	}

	if meta.Permission != "755" {
		t.Errorf("Permission = %q, want %q", meta.Permission, "755")
	}

	if meta.Owner != "root" {
		t.Errorf("Owner = %q, want %q", meta.Owner, "root")
	}
}

func TestClient_StatDirMetadata_Error(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("stat: cannot stat"), sshErr: errExit1}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := client.StatDirMetadata("/srv/data")
	if statErr == nil {
		t.Fatal("expected error from StatDirMetadata on SSH failure")
	}
}

// ---------------------------------------------------------------------------
// minimalFileInfo
// ---------------------------------------------------------------------------

func TestMinimalFileInfo(t *testing.T) {
	t.Parallel()

	info := minimalFileInfo{name: "compose.yml"}

	if info.Name() != "compose.yml" {
		t.Errorf("Name() = %q, want %q", info.Name(), "compose.yml")
	}

	if info.Size() != 0 {
		t.Errorf("Size() = %d, want 0", info.Size())
	}

	if info.Mode() != 0 {
		t.Errorf("Mode() = %v, want 0", info.Mode())
	}

	if !info.ModTime().IsZero() {
		t.Errorf("ModTime() should be zero, got %v", info.ModTime())
	}

	if info.IsDir() {
		t.Error("IsDir() should be false")
	}

	if info.Sys() != nil {
		t.Errorf("Sys() should be nil, got %v", info.Sys())
	}
}

// ---------------------------------------------------------------------------

func TestParseDirStatOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		wantErr bool
		want    *DirMetadata
	}{
		{
			name:   "standard output",
			output: "755 0 0 root root\n",
			want:   &DirMetadata{Permission: "755", OwnerID: "0", GroupID: "0", Owner: "root", Group: "root"},
		},
		{
			name:   "trimmed output",
			output: "  750 1000 50 app staff  ",
			want:   &DirMetadata{Permission: "750", OwnerID: "1000", GroupID: "50", Owner: "app", Group: "staff"},
		},
		{
			name:   "setuid permission",
			output: "3755 1000 1000 deploy deploy\n",
			want:   &DirMetadata{Permission: "3755", OwnerID: "1000", GroupID: "1000", Owner: "deploy", Group: "deploy"},
		},
		{
			name:    "too few fields",
			output:  "755 0 0 root",
			wantErr: true,
		},
		{
			name:    "too many fields",
			output:  "755 0 0 root wheel extra",
			wantErr: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseDirStatOutput(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Permission != tt.want.Permission {
				t.Errorf("Permission = %q, want %q", got.Permission, tt.want.Permission)
			}

			if got.Owner != tt.want.Owner {
				t.Errorf("Owner = %q, want %q", got.Owner, tt.want.Owner)
			}

			if got.Group != tt.want.Group {
				t.Errorf("Group = %q, want %q", got.Group, tt.want.Group)
			}

			if got.OwnerID != tt.want.OwnerID {
				t.Errorf("OwnerID = %q, want %q", got.OwnerID, tt.want.OwnerID)
			}

			if got.GroupID != tt.want.GroupID {
				t.Errorf("GroupID = %q, want %q", got.GroupID, tt.want.GroupID)
			}
		})
	}
}
