package remote

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_remote_interfaces.go -package=remote github.com/shiron-dev/melisia/tools/cmt/internal/remote RemoteClient,ClientFactory,CommandRunner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

var (
	errPathDoesNotExist     = errors.New("path does not exist")
	errExistenceUnknown     = errors.New("path existence unknown")
	errUnexpectedStatOutput = errors.New("unexpected stat output")
)

// ErrExistenceUnknown is returned by Stat when the path existence could not be
// determined (e.g. SSH connection failed). Callers should treat it as unknown
// and exit with failure at the end.
var ErrExistenceUnknown = errExistenceUnknown

type CommandRunner interface {
	SSHCombinedOutput(ctx context.Context, args ...string) ([]byte, error)
	SCPCombinedOutput(ctx context.Context, args ...string) ([]byte, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) SSHCombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh")
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = "ssh"
	copy(cmd.Args[1:], args)

	return cmd.CombinedOutput()
}

func (ExecCommandRunner) SCPCombinedOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "scp")
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = "scp"
	copy(cmd.Args[1:], args)

	return cmd.CombinedOutput()
}

type DirMetadata struct {
	Permission string
	Owner      string
	Group      string
	OwnerID    string
	GroupID    string
}

type RemoteClient interface {
	ReadFile(ctx context.Context, remotePath string) ([]byte, error)
	WriteFile(ctx context.Context, remotePath string, data []byte) error
	MkdirAll(ctx context.Context, dir string) error
	Remove(ctx context.Context, remotePath string) error
	Stat(ctx context.Context, remotePath string) (fs.FileInfo, error)
	StatDirMetadata(ctx context.Context, remotePath string) (*DirMetadata, error)
	ListFilesRecursive(ctx context.Context, dir string) ([]string, error)
	RunCommand(ctx context.Context, workdir, command string) (string, error)
	Close() error
}

type ClientFactory interface {
	NewClient(entry config.HostEntry) (RemoteClient, error)
}

type DefaultClientFactory struct {
	Runner CommandRunner
}

func (f DefaultClientFactory) NewClient(entry config.HostEntry) (RemoteClient, error) {
	runner := f.Runner
	if runner == nil {
		runner = ExecCommandRunner{}
	}

	return NewClientWithRunner(entry, runner)
}

var _ RemoteClient = (*Client)(nil)

type Client struct {
	host    config.HostEntry
	sshArgs []string
	scpArgs []string
	runner  CommandRunner
}

func NewClient(entry config.HostEntry) (*Client, error) {
	return NewClientWithRunner(entry, ExecCommandRunner{})
}

func NewClientWithRunner(entry config.HostEntry, runner CommandRunner) (*Client, error) {
	sshArgs, scpArgs := buildArgs(entry)
	slog.Debug("client created",
		"host", entry.Name,
		"sshArgs", strings.Join(sshArgs, " "),
		"scpArgs", strings.Join(scpArgs, " "),
	)

	return &Client{
		host:    entry,
		sshArgs: sshArgs,
		scpArgs: scpArgs,
		runner:  runner,
	}, nil
}

func (c *Client) Close() error { return nil }

func (c *Client) ReadFile(ctx context.Context, remotePath string) ([]byte, error) {
	out, err := c.runSSH(ctx, "cat "+shellQuote(remotePath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", remotePath, err)
	}

	return out, nil
}

func (c *Client) WriteFile(ctx context.Context, remotePath string, data []byte) error {
	dir := path.Dir(remotePath)

	err := c.MkdirAll(ctx, dir)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp("", "cmt-upload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()

	defer func() {
		_ = os.Remove(tmpPath)
	}()

	_, err = tmp.Write(data)
	if err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write temp file: %w", err)
	}

	closeErr := tmp.Close()
	if closeErr != nil {
		return fmt.Errorf("close temp file: %w", closeErr)
	}

	err = c.runSCP(ctx, tmpPath, remotePath)
	if err != nil {
		return fmt.Errorf("scp to %s: %w", remotePath, err)
	}

	return nil
}

func (c *Client) MkdirAll(ctx context.Context, dir string) error {
	_, err := c.runSSH(ctx, "mkdir -p "+shellQuote(dir))

	return err
}

func (c *Client) Remove(ctx context.Context, remotePath string) error {
	_, err := c.runSSH(ctx, "rm -f "+shellQuote(remotePath))

	return err
}

func (c *Client) Stat(ctx context.Context, remotePath string) (fs.FileInfo, error) {
	_, err := c.runSSH(ctx, "test -e "+shellQuote(remotePath))
	if err == nil {
		return minimalFileInfo{name: path.Base(remotePath)}, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil, fmt.Errorf("stat %s: %w", remotePath, errPathDoesNotExist)
	}

	return nil, fmt.Errorf("stat %s: %w", remotePath, errExistenceUnknown)
}

func (c *Client) StatDirMetadata(ctx context.Context, remotePath string) (*DirMetadata, error) {
	cmd := "stat -c '%a %u %g %U %G' " + shellQuote(remotePath)

	out, err := c.runSSH(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("stat metadata %s: %w", remotePath, err)
	}

	return ParseDirStatOutput(string(out))
}

func ParseDirStatOutput(output string) (*DirMetadata, error) {
	fields := strings.Fields(strings.TrimSpace(output))

	const expectedFields = 5

	if len(fields) != expectedFields {
		return nil, fmt.Errorf("%w: %q", errUnexpectedStatOutput, output)
	}

	return &DirMetadata{
		Permission: fields[0],
		OwnerID:    fields[1],
		GroupID:    fields[2],
		Owner:      fields[3],
		Group:      fields[4],
	}, nil
}

func (c *Client) ListFilesRecursive(ctx context.Context, dir string) ([]string, error) {
	out, err := c.runSSH(ctx, fmt.Sprintf(
		"find %s -type f 2>/dev/null || true", shellQuote(dir),
	))
	if err != nil {
		return nil, err
	}

	var files []string

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}

		rel := strings.TrimPrefix(line, dir+"/")
		if rel != line {
			files = append(files, rel)
		}
	}

	return files, nil
}

func (c *Client) RunCommand(ctx context.Context, workdir, command string) (string, error) {
	cmd := command
	if workdir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellQuote(workdir), command)
	}

	out, err := c.runSSH(ctx, cmd)

	return string(out), err
}

func (c *Client) runSSH(ctx context.Context, remoteCmd string) ([]byte, error) {
	const sshArgsPadding = 3

	args := make([]string, 0, len(c.sshArgs)+sshArgsPadding)
	args = append(args, c.sshArgs...)
	args = append(args, c.host.Host, "--", remoteCmd)

	slog.Debug("running ssh", "command", "ssh "+strings.Join(args, " "))

	out, err := c.runner.SSHCombinedOutput(ctx, args...)
	if err != nil {
		return out, fmt.Errorf("ssh %s: %w\n%s", c.host.Name, err, out)
	}

	return out, nil
}

func (c *Client) runSCP(ctx context.Context, localPath, remotePath string) error {
	dest := fmt.Sprintf("%s@%s:%s", c.host.User, c.host.Host, remotePath)

	const scpArgsPadding = 2

	args := make([]string, 0, len(c.scpArgs)+scpArgsPadding)
	args = append(args, c.scpArgs...)
	args = append(args, localPath, dest)

	slog.Debug("running scp")

	out, err := c.runner.SCPCombinedOutput(ctx, args...)
	if err != nil {
		return fmt.Errorf("scp to %s: %w\n%s", dest, err, out)
	}

	return nil
}

func buildArgs(entry config.HostEntry) ([]string, []string) {
	var (
		sshArgs []string
		scpArgs []string
	)

	commonOpts := buildCommonOptions(entry)

	sshArgs = append(sshArgs, commonOpts...)
	if entry.Port != 0 && entry.Port != 22 {
		sshArgs = append(sshArgs, "-p", strconv.Itoa(entry.Port))
	}

	if entry.User != "" {
		sshArgs = append(sshArgs, "-l", entry.User)
	}

	scpArgs = append(scpArgs, commonOpts...)
	if entry.Port != 0 && entry.Port != 22 {
		scpArgs = append(scpArgs, "-P", strconv.Itoa(entry.Port))
	}

	return sshArgs, scpArgs
}

// defaultConnectTimeoutSeconds bounds the SSH/SCP connection phase. Without it a
// dead or unreachable host stalls each command for the OS default (often well
// over a minute), which makes a single bad host hold up the whole plan. Command
// execution after a successful connect is bounded by the caller's context, not
// this option, so long operations (image pulls during apply) are unaffected.
const defaultConnectTimeoutSeconds = 10

func buildCommonOptions(entry config.HostEntry) []string {
	commonOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=" + strconv.Itoa(defaultConnectTimeoutSeconds),
	}

	if entry.ProxyCommand != "" {
		commonOpts = append(commonOpts, "-o", "ProxyCommand="+entry.ProxyCommand)
	}

	if entry.IdentityAgent != "" {
		commonOpts = append(commonOpts, "-o", "IdentityAgent="+escapeSSHOptionValue(entry.IdentityAgent))
	}

	for _, keyPath := range entry.IdentityFiles {
		commonOpts = append(commonOpts, "-i", keyPath)
	}

	if entry.SSHKeyPath != "" && len(entry.IdentityFiles) == 0 {
		commonOpts = append(commonOpts, "-i", entry.SSHKeyPath)
	}

	return commonOpts
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func escapeSSHOptionValue(s string) string {
	return strings.ReplaceAll(s, " ", `\ `)
}

type minimalFileInfo struct{ name string }

func (m minimalFileInfo) Name() string       { return m.name }
func (m minimalFileInfo) Size() int64        { return 0 }
func (m minimalFileInfo) Mode() fs.FileMode  { return 0 }
func (m minimalFileInfo) ModTime() time.Time { return time.Time{} }
func (m minimalFileInfo) IsDir() bool        { return false }
func (m minimalFileInfo) Sys() any           { return nil }
