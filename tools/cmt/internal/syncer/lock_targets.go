package syncer

import (
	"errors"
	"fmt"
	"path"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/shiron-dev/melisia/tools/cmt/internal/lock"
)

// ResolveLockTargets enumerates the per-project lock targets that an operation
// would touch under the given host/project filters. It mirrors the plan's
// project discovery and SSH resolution, but skips the expensive remote diff so
// it can run before a lock is held.
func ResolveLockTargets(
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	deps PlanDependencies,
) ([]lock.Target, error) {
	return resolveLockTargets(cfg, hostFilter, projectFilter, deps, false)
}

// ResolveLockTargetsLenient behaves like ResolveLockTargets but skips hosts
// whose config matches none of the requested projects instead of failing. It is
// used by force-unlock --all, where "release grafana everywhere" must not abort
// just because some host never runs grafana.
func ResolveLockTargetsLenient(
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	deps PlanDependencies,
) ([]lock.Target, error) {
	return resolveLockTargets(cfg, hostFilter, projectFilter, deps, true)
}

func resolveLockTargets(
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	deps PlanDependencies,
	skipUnmatchedHosts bool,
) ([]lock.Target, error) {
	_, sshResolver, _ := resolvePlanDependencies(deps)

	allProjects, err := config.DiscoverProjects(cfg.BasePath)
	if err != nil {
		return nil, err
	}

	projects := config.FilterProjects(allProjects, projectFilter)
	if len(projects) == 0 {
		return nil, fmt.Errorf("%w (filter: %v)", errNoProjectsFound, projectFilter)
	}

	hosts := config.FilterHosts(cfg.Hosts, hostFilter)
	if len(hosts) == 0 {
		return nil, fmt.Errorf("%w %v", errNoHostsMatched, hostFilter)
	}

	var targets []lock.Target

	for _, host := range hosts {
		hostTargets, err := resolveHostLockTargets(cfg, host, projects, sshResolver)
		if err != nil {
			if skipUnmatchedHosts && errors.Is(err, errNoHostProjectsMatched) {
				continue
			}

			return nil, err
		}

		targets = append(targets, hostTargets...)
	}

	return targets, nil
}

func resolveHostLockTargets(
	cfg *config.CmtConfig,
	host config.HostEntry,
	projects []string,
	sshResolver config.SSHConfigResolver,
) ([]lock.Target, error) {
	hostCfg, found, err := loadHostConfig(cfg.BasePath, host.Name)
	if err != nil {
		return nil, fmt.Errorf("loading host config for %s: %w", host.Name, err)
	}

	if !found {
		hostCfg = nil
	}

	// Match projects before resolving SSH: a host that runs none of the requested
	// projects can then be skipped (by the lenient caller) without paying the
	// ssh -G cost or failing on that host's missing/broken SSH config.
	hostProjects, err := filterHostProjects(host, hostCfg, projects)
	if err != nil {
		return nil, err
	}

	resolvedHost := host

	err = resolveHostSSHConfig(cfg.BasePath, &resolvedHost, hostCfg, sshResolver)
	if err != nil {
		return nil, fmt.Errorf("resolving SSH config for %s: %w", host.Name, err)
	}

	targets := make([]lock.Target, 0, len(hostProjects))

	for _, project := range hostProjects {
		resolved := config.ResolveProjectConfig(cfg.Defaults, hostCfg, project)
		if resolved.RemotePath == "" {
			return nil, fmt.Errorf("%w for host %q, project %q", errRemotePathNotSet, host.Name, project)
		}

		remoteDir := path.Join(resolved.RemotePath, project)
		targets = append(targets, lock.Target{
			Host:      resolvedHost,
			Project:   project,
			RemoteDir: remoteDir,
			LockPath:  path.Join(remoteDir, lock.LockFileName),
		})
	}

	return targets, nil
}
