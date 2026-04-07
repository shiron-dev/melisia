package remote

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_remote_interfaces.go -package=remote cmt/internal/remote RemoteClient,ClientFactory,CommandRunner

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

	"cmt/internal/config"
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
	SSHCombinedOutput(args ...string) ([]byte, error)
	SCPCombinedOutput(args ...string) ([]byte, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) SSHCombinedOutput(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(context.Background(), "ssh")
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = "ssh"
	copy(cmd.Args[1:], args)

	return cmd.CombinedOutput()
}

func (ExecCommandRunner) SCPCombinedOutput(args ...string) ([]byte, error) {
	cmd := exec.CommandContext(context.Background(), "scp")
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
	ReadFile(remotePath string) ([]byte, error)
	WriteFile(remotePath string, data []byte) error
	MkdirAll(dir string) error
	Remove(remotePath string) error
	Stat(remotePath string) (fs.FileInfo, error)
	StatDirMetadata(remotePath string) (*DirMetadata, error)
	ListFilesRecursive(dir string) ([]string, error)
	RunCommand(workdir, command string) (string, error)
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

func (c *Client) ReadFile(remotePath string) ([]byte, error) {
	out, err := c.runSSH("cat " + shellQuote(remotePath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", remotePath, err)
	}

	return out, nil
}

func (c *Client) WriteFile(remotePath string, data []byte) error {
	dir := path.Dir(remotePath)

	err := c.MkdirAll(dir)
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

	if err = os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	err = c.runSCP(tmpPath, remotePath)
	if err != nil {
		return fmt.Errorf("scp to %s: %w", remotePath, err)
	}

	return nil
}

func (c *Client) MkdirAll(dir string) error {
	_, err := c.runSSH("mkdir -p " + shellQuote(dir))

	return err
}

func (c *Client) Remove(remotePath string) error {
	_, err := c.runSSH("rm -f " + shellQuote(remotePath))

	return err
}

func (c *Client) Stat(remotePath string) (fs.FileInfo, error) {
	_, err := c.runSSH("test -e " + shellQuote(remotePath))
	if err == nil {
		return minimalFileInfo{name: path.Base(remotePath)}, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil, fmt.Errorf("stat %s: %w", remotePath, errPathDoesNotExist)
	}

	return nil, fmt.Errorf("stat %s: %w", remotePath, errExistenceUnknown)
}

func (c *Client) StatDirMetadata(remotePath string) (*DirMetadata, error) {
	cmd := "stat -c '%a %u %g %U %G' " + shellQuote(remotePath)

	out, err := c.runSSH(cmd)
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

func (c *Client) ListFilesRecursive(dir string) ([]string, error) {
	out, err := c.runSSH(fmt.Sprintf(
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

func (c *Client) RunCommand(workdir, command string) (string, error) {
	cmd := command
	if workdir != "" {
		cmd = fmt.Sprintf("cd %s && %s", shellQuote(workdir), command)
	}

	out, err := c.runSSH(cmd)

	return string(out), err
}

func (c *Client) runSSH(remoteCmd string) ([]byte, error) {
	const sshArgsPadding = 3

	args := make([]string, 0, len(c.sshArgs)+sshArgsPadding)
	args = append(args, c.sshArgs...)
	args = append(args, c.host.Host, "--", remoteCmd)

	slog.Debug("running ssh", "command", "ssh "+strings.Join(args, " "))

	out, err := c.runner.SSHCombinedOutput(args...)
	if err != nil {
		return out, fmt.Errorf("ssh %s: %w\n%s", c.host.Name, err, out)
	}

	return out, nil
}

func (c *Client) runSCP(localPath, remotePath string) error {
	dest := fmt.Sprintf("%s@%s:%s", c.host.User, c.host.Host, remotePath)

	const scpArgsPadding = 2

	args := make([]string, 0, len(c.scpArgs)+scpArgsPadding)
	args = append(args, c.scpArgs...)
	args = append(args, localPath, dest)

	slog.Debug("running scp")

	out, err := c.runner.SCPCombinedOutput(args...)
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

func buildCommonOptions(entry config.HostEntry) []string {
	commonOpts := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",
	}

	if entry.ProxyCommand != "" {
		commonOpts = append(commonOpts, "-o", "ProxyCommand="+entry.ProxyCommand)
	}

	if entry.IdentityAgent != "" && entry.IdentityAgent != "none" {
		commonOpts = append(commonOpts, "-o", `IdentityAgent="`+entry.IdentityAgent+`"`)
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

type minimalFileInfo struct{ name string }

func (m minimalFileInfo) Name() string       { return m.name }
func (m minimalFileInfo) Size() int64        { return 0 }
func (m minimalFileInfo) Mode() fs.FileMode  { return 0 }
func (m minimalFileInfo) ModTime() time.Time { return time.Time{} }
func (m minimalFileInfo) IsDir() bool        { return false }
func (m minimalFileInfo) Sys() any           { return nil }
