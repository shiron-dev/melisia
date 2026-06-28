package remote

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

var (
	errMkdirFailed = errors.New("mkdir failed")
	errSCPFailed   = errors.New("scp failed")
)

// requireBinary は指定バイナリが PATH に無ければテストをスキップします。
func requireBinary(t *testing.T, name string) {
	t.Helper()

	_, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not available", name)
	}
}

// makeExitError は実際に終了コード code で終了するプロセスを起動し、
// その *exec.ExitError を返します。Stat の終了コード分岐を検証するために使います。
func makeExitError(t *testing.T, code int) error {
	t.Helper()

	//nolint:gosec // G204: テストが終了コードを再現するために意図的にシェルを起動する。
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit "+strconv.Itoa(code))

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-nil error for exit %d", code)
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T", err)
	}

	return err
}

// ---------------------------------------------------------------------------
// shellQuote: コマンドインジェクション対策 (セキュリティ上最重要)
// ---------------------------------------------------------------------------

// TestShellQuote_InjectionRoundTrip は、シェルのメタ文字を含む入力を
// shellQuote した結果が POSIX シェルで「ちょうど元の文字列1つ」に
// 復元されること（=展開・コマンド実行が起きないこと）を実プロセスで検証します。
//
// 重要: 万一 shellQuote が壊れてペイロードが sh により実行されても CI 環境を
// 破壊しないよう、(1) 埋め込むコマンドは echo / 無害なリダイレクトのみとし、
// rm・reboot・touch といった破壊的・永続的な操作は一切使わない。
// (2) 作業ディレクトリを使い捨ての一時ディレクトリに固定し、リダイレクトや
// グロブ展開が漏れても副作用をその中に閉じ込める。
// 引用が正しければ各ペイロードはリテラル出力となり (round-trip 一致)、
// 壊れていれば出力が一致せずテストが失敗して回帰を検知する。
func TestShellQuote_InjectionRoundTrip(t *testing.T) {
	t.Parallel()

	requireBinary(t, "sh")

	// 副作用を閉じ込める使い捨てディレクトリ。
	sandbox := t.TempDir()

	// シェルのメタ文字を網羅しつつ、各コマンドは無害なものに留める。
	payloads := []string{
		`; echo injected`, // コマンド区切り
		`$(echo sub)`,     // コマンド置換
		"`echo backtick`", // バッククォート置換
		`&& echo and`,     // AND リスト
		`|| echo or`,      // OR リスト
		`x | echo pipe`,   // パイプ
		`> sentinel.txt`,  // リダイレクト (漏れても sandbox 内に閉じる)
		`$HOME`,           // 変数展開
		`${PATH}`,         // 変数展開 (波括弧)
		`"double"`,        // ダブルクォート
		`it's a 'trap'`,   // シングルクォート (エスケープの肝)
		`a\b`,             // バックスラッシュ
		`new
line`, // 改行
		`*`,   // グロブ
		`-rf`, // 先頭ハイフン
		``,    // 空文字列
	}

	for _, input := range payloads {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			quoted := shellQuote(input)

			// printf %s <quoted> は、引数が安全に1トークン化されていれば
			// 元の文字列をそのまま出力する。展開やコマンド実行が起きれば一致しない。
			//nolint:gosec // G204: 引用の安全性を実シェルで検証するのが目的。無害なペイロードのみ使用。
			cmd := exec.CommandContext(context.Background(), "sh", "-c", "printf %s "+quoted)
			cmd.Dir = sandbox // リダイレクト/グロブが漏れても使い捨てディレクトリに閉じ込める

			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("sh failed for input %q (quoted %q): %v\noutput: %s", input, quoted, err, out)
			}

			if string(out) != input {
				t.Errorf("round-trip mismatch: input %q quoted %q -> %q", input, quoted, string(out))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// WriteFile: MkdirAll + SCP の引数組み立て経路
// ---------------------------------------------------------------------------

func TestClient_WriteFile_Success(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil, scpErr: nil}

	entry := config.HostEntry{Name: "s1", Host: "h1", User: "deploy"}

	client, err := NewClientWithRunner(entry, runner)
	if err != nil {
		t.Fatal(err)
	}

	writeErr := client.WriteFile(context.Background(), "/srv/app/compose.yml", []byte("hello"))
	if writeErr != nil {
		t.Fatalf("WriteFile: %v", writeErr)
	}

	// 親ディレクトリの mkdir -p が SSH で実行される。
	sshCmd := strings.Join(runner.capturedSSHArgs, " ")
	if !strings.Contains(sshCmd, "mkdir -p '/srv/app'") {
		t.Errorf("expected mkdir -p '/srv/app', got: %s", sshCmd)
	}

	// SCP の宛先は user@host:remotePath 形式で組み立てられる。
	if len(runner.capturedSCPArgs) < 2 {
		t.Fatalf("expected scp args, got: %v", runner.capturedSCPArgs)
	}

	dest := runner.capturedSCPArgs[len(runner.capturedSCPArgs)-1]
	if dest != "deploy@h1:/srv/app/compose.yml" {
		t.Errorf("scp dest = %q, want %q", dest, "deploy@h1:/srv/app/compose.yml")
	}
}

func TestClient_WriteFile_MkdirError(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("permission denied"), sshErr: errMkdirFailed}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	writeErr := client.WriteFile(context.Background(), "/srv/app/compose.yml", []byte("hello"))
	if writeErr == nil {
		t.Fatal("WriteFile should fail when MkdirAll fails")
	}

	if !strings.Contains(writeErr.Error(), "mkdir") {
		t.Errorf("error should mention mkdir, got: %v", writeErr)
	}
}

func TestClient_WriteFile_SCPError(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil, scpErr: errSCPFailed}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1", User: "deploy"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	writeErr := client.WriteFile(context.Background(), "/srv/app/compose.yml", []byte("hello"))
	if writeErr == nil {
		t.Fatal("WriteFile should fail when SCP fails")
	}

	if !strings.Contains(writeErr.Error(), "scp") {
		t.Errorf("error should mention scp, got: %v", writeErr)
	}
}

// TestClient_WriteFile_TempFileError は一時ファイル作成に失敗した場合
// (TMPDIR が存在しない) にエラーを返すことを検証します。
// t.Setenv を使うため t.Parallel() は呼べません。
func TestClient_WriteFile_TempFileError(t *testing.T) {
	t.Setenv("TMPDIR", "/nonexistent-cmt-tmpdir/does/not/exist")

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1", User: "deploy"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	writeErr := client.WriteFile(context.Background(), "/srv/app/compose.yml", []byte("hello"))
	if writeErr == nil {
		t.Fatal("WriteFile should fail when temp file cannot be created")
	}

	if !strings.Contains(writeErr.Error(), "temp file") {
		t.Errorf("error should mention temp file, got: %v", writeErr)
	}
}

// ---------------------------------------------------------------------------
// runSCP: 宛先・オプションの引数組み立て (ポート/鍵込み)
// ---------------------------------------------------------------------------

func TestClient_runSCP_ArgAssembly(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{}

	entry := config.HostEntry{
		Name:          "s1",
		Host:          "h1",
		User:          "deploy",
		Port:          2222,
		IdentityFiles: []string{"/keys/id_ed25519"},
	}

	client, err := NewClientWithRunner(entry, runner)
	if err != nil {
		t.Fatal(err)
	}

	scpErr := client.runSCP(context.Background(), "/tmp/local-file", "/srv/app/compose.yml")
	if scpErr != nil {
		t.Fatalf("runSCP: %v", scpErr)
	}

	args := runner.capturedSCPArgs

	// カスタムポートは大文字 -P で渡る (ssh の -p とは異なる)。
	if !containsSeq(args, "-P", "2222") {
		t.Errorf("scp args should contain -P 2222, got %v", args)
	}

	if !containsSeq(args, "-i", "/keys/id_ed25519") {
		t.Errorf("scp args should contain -i /keys/id_ed25519, got %v", args)
	}

	// 末尾2要素は localPath, user@host:remotePath の順。
	const tailLen = 2
	if len(args) < tailLen {
		t.Fatalf("expected at least %d args, got %v", tailLen, args)
	}

	if local := args[len(args)-tailLen]; local != "/tmp/local-file" {
		t.Errorf("scp local = %q, want %q", local, "/tmp/local-file")
	}

	if dest := args[len(args)-1]; dest != "deploy@h1:/srv/app/compose.yml" {
		t.Errorf("scp dest = %q, want %q", dest, "deploy@h1:/srv/app/compose.yml")
	}
}

func TestClient_runSCP_Error(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{scpErr: errSCPFailed}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1", User: "deploy"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	scpErr := client.runSCP(context.Background(), "/tmp/local-file", "/srv/app/compose.yml")
	if scpErr == nil {
		t.Fatal("runSCP should return error when SCP fails")
	}
}

// ---------------------------------------------------------------------------
// Stat: 終了コードによる分岐
// ---------------------------------------------------------------------------

func TestClient_Stat_NotExist(t *testing.T) {
	t.Parallel()

	requireBinary(t, "sh")

	// test -e がパス不在で終了コード1を返すケースを再現する。
	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: makeExitError(t, 1)}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := client.Stat(context.Background(), "/srv/data/missing.txt")
	if statErr == nil {
		t.Fatal("expected error for missing path")
	}

	if !errors.Is(statErr, errPathDoesNotExist) {
		t.Errorf("expected errPathDoesNotExist, got %v", statErr)
	}
}

func TestClient_Stat_OtherExitCodeIsUnknown(t *testing.T) {
	t.Parallel()

	requireBinary(t, "sh")

	// 終了コード1以外 (例: 接続エラーの255系) は「不明」として扱う。
	runner := &mockCommandRunner{sshOutput: []byte("ssh: connect failed"), sshErr: makeExitError(t, 2)}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, statErr := client.Stat(context.Background(), "/srv/data/file.txt")
	if statErr == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(statErr, errExistenceUnknown) {
		t.Errorf("expected errExistenceUnknown for exit code 2, got %v", statErr)
	}
}

// ---------------------------------------------------------------------------
// ListFilesRecursive: SSH エラー経路
// ---------------------------------------------------------------------------

func TestClient_ListFilesRecursive_Error(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte("connection refused"), sshErr: errSSHFailed}

	client, err := NewClientWithRunner(config.HostEntry{Name: "s1", Host: "h1"}, runner)
	if err != nil {
		t.Fatal(err)
	}

	_, listErr := client.ListFilesRecursive(context.Background(), "/srv/grafana")
	if listErr == nil {
		t.Fatal("ListFilesRecursive should return error on SSH failure")
	}
}

// ---------------------------------------------------------------------------
// コンストラクタ / ファクトリ
// ---------------------------------------------------------------------------

func TestNewClient(t *testing.T) {
	t.Parallel()

	client, err := NewClient(config.HostEntry{Name: "s1", Host: "h1", Port: 2222, User: "deploy"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	// 既定の runner として ExecCommandRunner が設定される。
	if _, ok := client.runner.(ExecCommandRunner); !ok {
		t.Errorf("expected ExecCommandRunner, got %T", client.runner)
	}
}

func TestDefaultClientFactory_NewClient_WithRunner(t *testing.T) {
	t.Parallel()

	runner := &mockCommandRunner{sshOutput: []byte(""), sshErr: nil}
	factory := DefaultClientFactory{Runner: runner}

	client, err := factory.NewClient(config.HostEntry{Name: "s1", Host: "h1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// 指定した runner が実際に使われることを、コマンド実行で確認する。
	mkdirErr := client.MkdirAll(context.Background(), "/srv/data")
	if mkdirErr != nil {
		t.Fatalf("MkdirAll: %v", mkdirErr)
	}

	if len(runner.capturedSSHArgs) == 0 {
		t.Error("expected provided runner to capture SSH args")
	}
}

func TestDefaultClientFactory_NewClient_NilRunnerFallsBack(t *testing.T) {
	t.Parallel()

	factory := DefaultClientFactory{}

	client, err := factory.NewClient(config.HostEntry{Name: "s1", Host: "h1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	concrete, ok := client.(*Client)
	if !ok {
		t.Fatalf("expected *Client, got %T", client)
	}

	if _, ok := concrete.runner.(ExecCommandRunner); !ok {
		t.Errorf("expected fallback to ExecCommandRunner, got %T", concrete.runner)
	}
}

// ---------------------------------------------------------------------------
// ExecCommandRunner: 実バイナリへの引数受け渡し (ネットワーク非依存の経路のみ)
// ---------------------------------------------------------------------------

func TestExecCommandRunner_SSHCombinedOutput(t *testing.T) {
	t.Parallel()

	requireBinary(t, "ssh")

	// `ssh -V` はバージョンを出力して即終了する (ネットワークアクセスなし)。
	out, err := ExecCommandRunner{}.SSHCombinedOutput(context.Background(), "-V")
	if err != nil {
		t.Fatalf("ssh -V failed: %v\n%s", err, out)
	}
}

func TestExecCommandRunner_SCPCombinedOutput(t *testing.T) {
	t.Parallel()

	requireBinary(t, "scp")

	// 引数なしの scp は usage を出して即終了する (ネットワークアクセスなし)。
	// ここでは引数の受け渡しが panic せず戻ることだけを確認する。
	out, _ := ExecCommandRunner{}.SCPCombinedOutput(context.Background())
	_ = out
}
