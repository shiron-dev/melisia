package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var ErrHostConfigNotFound = errors.New("host config not found")

var (
	ErrBasePathRequired = errors.New("basePath is required")
	ErrHostRequired     = errors.New("at least one host is required")
)

func LoadCmtConfig(configPath string) (*CmtConfig, error) {
	cleanConfigPath := filepath.Clean(configPath)

	data, err := os.ReadFile(cleanConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", cleanConfigPath, err)
	}

	var cfg CmtConfig

	errUnmarshal := yaml.Unmarshal(data, &cfg)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing config %s: %w", configPath, errUnmarshal)
	}

	if cfg.BasePath == "" {
		return nil, fmt.Errorf("%w in %s", ErrBasePathRequired, cleanConfigPath)
	}

	if len(cfg.Hosts) == 0 {
		return nil, fmt.Errorf("%w in %s", ErrHostRequired, cleanConfigPath)
	}

	if !filepath.IsAbs(cfg.BasePath) {
		configDir := filepath.Dir(cleanConfigPath)

		abs, err := filepath.Abs(filepath.Join(configDir, cfg.BasePath))
		if err != nil {
			return nil, fmt.Errorf("resolving basePath: %w", err)
		}

		cfg.BasePath = abs
	}

	return &cfg, nil
}

func LoadHostConfig(basePath, hostName string) (*HostConfig, error) {
	hostConfigPath := filepath.Join(basePath, "hosts", hostName, "host.yml")
	hostConfigPath = filepath.Clean(hostConfigPath)

	data, err := os.ReadFile(hostConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrHostConfigNotFound
		}

		return nil, fmt.Errorf("reading %s: %w", hostConfigPath, err)
	}

	hostConfig := new(HostConfig)
	hostConfig.SSHConfig = ""
	hostConfig.RemotePath = ""
	hostConfig.PostSyncCommand = ""
	hostConfig.Projects = nil

	errUnmarshal := yaml.Unmarshal(data, hostConfig)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing %s: %w", hostConfigPath, errUnmarshal)
	}

	for projectName, projectConfig := range hostConfig.Projects {
		if projectConfig == nil {
			continue
		}

		err := ValidateDirConfigs(projectConfig.Dirs)
		if err != nil {
			return nil, fmt.Errorf("project %s in %s: %w", projectName, hostConfigPath, err)
		}
	}

	return hostConfig, nil
}

func DiscoverProjects(basePath string) ([]string, error) {
	dir := filepath.Join(basePath, "projects")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading projects directory %s: %w", dir, err)
	}

	var projects []string

	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}

	return projects, nil
}

func FilterHosts(hosts []HostEntry, filter []string) []HostEntry {
	if len(filter) == 0 {
		return hosts
	}

	set := make(map[string]bool, len(filter))
	for _, f := range filter {
		set[f] = true
	}

	var out []HostEntry

	for _, h := range hosts {
		if set[h.Name] {
			out = append(out, h)
		}
	}

	return out
}

func FilterProjects(projects []string, filter []string) []string {
	if len(filter) == 0 {
		return projects
	}

	set := make(map[string]bool, len(filter))
	for _, f := range filter {
		set[f] = true
	}

	var out []string

	for _, projectName := range projects {
		if set[projectName] {
			out = append(out, projectName)
		}
	}

	return out
}
