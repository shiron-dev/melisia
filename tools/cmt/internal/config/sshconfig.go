package config

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_ssh_interfaces.go -package=config github.com/shiron-dev/melisia/tools/cmt/internal/config SSHConfigRunner,SSHConfigResolver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type SSHConfigRunner interface {
	SSHOutput(ctx context.Context, args ...string) ([]byte, error)
}

type ExecSSHConfigRunner struct{}

func (ExecSSHConfigRunner) SSHOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ssh")
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = "ssh"
	copy(cmd.Args[1:], args)

	return cmd.Output()
}

type SSHConfigResolver interface {
	Resolve(ctx context.Context, entry *HostEntry, sshConfigPath, hostDir string) error
}

type DefaultSSHConfigResolver struct {
	Runner SSHConfigRunner
}

func (r DefaultSSHConfigResolver) Resolve(ctx context.Context, entry *HostEntry, sshConfigPath, hostDir string) error {
	runner := r.Runner
	if runner == nil {
		runner = ExecSSHConfigRunner{}
	}

	return ResolveSSHConfigWithRunner(ctx, entry, sshConfigPath, hostDir, runner)
}

func ResolveSSHConfig(ctx context.Context, entry *HostEntry, sshConfigPath, hostDir string) error {
	return ResolveSSHConfigWithRunner(ctx, entry, sshConfigPath, hostDir, ExecSSHConfigRunner{})
}

func ResolveSSHConfigWithRunner(
	ctx context.Context,
	entry *HostEntry,
	sshConfigPath, hostDir string,
	runner SSHConfigRunner,
) error {
	originalHost := entry.Host
	args := buildSSHGArgs(entry, sshConfigPath, hostDir)

	slog.Debug("running ssh -G", "command", "ssh "+strings.Join(args, " "), "host", entry.Name)

	out, err := runner.SSHOutput(ctx, args...)
	if err != nil {
		exitErr := new(exec.ExitError)
		if errors.As(err, &exitErr) {
			return fmt.Errorf("ssh -G %s: %w\nstderr: %s", entry.Host, err, exitErr.Stderr)
		}

		return fmt.Errorf("ssh -G %s: %w", entry.Host, err)
	}

	singleValues, multiValues := parseSSHGOutput(string(out))
	applyResolvedConnectionFields(entry, singleValues)
	applyResolvedProxyAndIdentity(entry, singleValues, multiValues, originalHost)

	slog.Debug("ssh -G resolved",
		"host", entry.Name,
		"hostname", entry.Host,
		"user", entry.User,
		"port", entry.Port,
		"proxycommand", entry.ProxyCommand,
		"identityfiles", entry.IdentityFiles,
		"identityagent", entry.IdentityAgent,
	)

	return nil
}

func buildSSHGArgs(entry *HostEntry, sshConfigPath, hostDir string) []string {
	arguments := []string{"-G"}

	if sshConfigPath != "" {
		if !filepath.IsAbs(sshConfigPath) {
			sshConfigPath = filepath.Join(hostDir, sshConfigPath)
		}

		arguments = append(arguments, "-F", sshConfigPath)
	}

	if entry.User != "" {
		arguments = append(arguments, "-l", entry.User)
	}

	if entry.Port != 0 {
		arguments = append(arguments, "-p", strconv.Itoa(entry.Port))
	}

	arguments = append(arguments, entry.Host)

	return arguments
}

func applyResolvedConnectionFields(entry *HostEntry, singleValues map[string]string) {
	if hostname, ok := singleValues["hostname"]; ok {
		entry.Host = hostname
	}

	if entry.User == "" {
		if resolvedUser, ok := singleValues["user"]; ok {
			entry.User = resolvedUser
		}
	}

	if entry.Port == 0 {
		if portValue, ok := singleValues["port"]; ok {
			parsedPort, err := strconv.Atoi(portValue)
			if err == nil && parsedPort > 0 {
				entry.Port = parsedPort
			}
		}
	}

	if entry.Port == 0 {
		entry.Port = 22
	}
}

func applyResolvedProxyAndIdentity(
	entry *HostEntry,
	singleValues map[string]string,
	multiValues map[string][]string,
	originalHost string,
) {
	if proxyCommand, ok := singleValues["proxycommand"]; ok && proxyCommand != "none" {
		entry.ProxyCommand = expandProxyPlaceholders(
			proxyCommand, entry.Host, entry.Port, entry.User, originalHost,
		)
	}

	if identityFiles, ok := multiValues["identityfile"]; ok {
		expandedIdentityFiles := make([]string, len(identityFiles))
		for index, identityFile := range identityFiles {
			expandedIdentityFiles[index] = expandProxyPlaceholders(
				identityFile, entry.Host, entry.Port, entry.User, originalHost,
			)
		}

		entry.IdentityFiles = expandedIdentityFiles
	}

	if identityAgent, ok := singleValues["identityagent"]; ok {
		entry.IdentityAgent = identityAgent
	}
}

func parseSSHGOutput(output string) (map[string]string, map[string][]string) {
	singleValues := make(map[string]string)
	multiValues := make(map[string][]string)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), " ")
		if !ok {
			continue
		}

		key = strings.ToLower(key)
		singleValues[key] = value
		multiValues[key] = append(multiValues[key], value)
	}

	return singleValues, multiValues
}

func expandProxyPlaceholders(cmd, hostname string, port int, user, originalHost string) string {
	cmd = strings.ReplaceAll(cmd, "%%", "\x00")
	cmd = strings.ReplaceAll(cmd, "%h", hostname)
	cmd = strings.ReplaceAll(cmd, "%p", strconv.Itoa(port))
	cmd = strings.ReplaceAll(cmd, "%r", user)
	cmd = strings.ReplaceAll(cmd, "%n", originalHost)
	cmd = strings.ReplaceAll(cmd, "\x00", "%")

	return cmd
}
