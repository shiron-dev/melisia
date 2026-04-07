package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"cmt/internal/config"
	"cmt/internal/remote"

	"github.com/pmezard/go-difflib/difflib"
)

type PlanDependencies struct {
	ClientFactory  remote.ClientFactory
	SSHResolver    config.SSHConfigResolver
	LocalRunner    LocalCommandRunner
	ProgressWriter io.Writer
}

var (
	errNoProjectsFound  = errors.New("no projects found")
	errNoHostsMatched   = errors.New("no hosts matched filter")
	errRemotePathNotSet = errors.New("remotePath is not set")
)

type LocalCommandRunner interface {
	Run(name string, args []string, workdir string) (string, error)
}

type ExecLocalCommandRunner struct{}

func (ExecLocalCommandRunner) Run(name string, args []string, workdir string) (string, error) {
	cmd := exec.CommandContext(context.Background(), name)
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = name
	copy(cmd.Args[1:], args)
	cmd.Dir = workdir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%w", err)
	}

	return string(output), nil
}

type ActionType int

const (
	ActionUnchanged ActionType = iota
	ActionAdd
	ActionModify
	ActionDelete
)

const labelUnchanged = "unchanged"

func (a ActionType) String() string {
	switch a {
	case ActionUnchanged:
		return labelUnchanged
	case ActionAdd:
		return "add"
	case ActionModify:
		return "modify"
	case ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

func (a ActionType) Symbol() string {
	switch a {
	case ActionUnchanged:
		return "="
	case ActionAdd:
		return "+"
	case ActionModify:
		return "~"
	case ActionDelete:
		return "-"
	default:
		return "?"
	}
}

type SyncPlan struct {
	HostPlans []HostPlan
}

type HostPlan struct {
	Host     config.HostEntry
	Projects []ProjectPlan
}

type ComposeActionType int

const (
	ComposeNoChange ComposeActionType = iota
	ComposeStartServices
	ComposeRecreateServices
	ComposeStopServices
)

type ComposePlan struct {
	DesiredAction string
	ActionType    ComposeActionType
	Services      []string
}

func (c *ComposePlan) HasChanges() bool {
	return c != nil && c.ActionType != ComposeNoChange && len(c.Services) > 0
}

type ProjectPlan struct {
	ProjectName     string
	RemoteDir       string
	PostSyncCommand string
	ComposeAction   string
	RemoveOrphans   bool
	Compose         *ComposePlan
	Dirs            []DirPlan
	Files           []FilePlan
}

func (pp *ProjectPlan) HasChanges() bool {
	for _, f := range pp.Files {
		if f.Action != ActionUnchanged {
			return true
		}
	}

	for _, d := range pp.Dirs {
		if d.Action != ActionUnchanged {
			return true
		}
	}

	return pp.Compose.HasChanges()
}

type DirPlan struct {
	RelativePath     string
	RemotePath       string
	Exists           bool
	ExistenceUnknown bool
	Action           ActionType
	Permission       string
	Owner            string
	Group            string
	Become           bool
	BecomeUser       string
	Recursive        bool
	ActualPermission string
	ActualOwner      string
	ActualGroup      string
	NeedsPermChange  bool
	NeedsOwnerChange bool
}

type FilePlan struct {
	RelativePath string
	LocalPath    string
	RemotePath   string
	Action       ActionType
	LocalData    []byte
	RemoteData   []byte
	Diff         string
	MaskHints    []MaskHint
}

type Manifest struct {
	ManagedFiles []string              `json:"managedFiles"`
	MaskHints    map[string][]MaskHint `json:"maskHints,omitempty"`
}

type MaskHint struct {
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix,omitempty"`
}

const manifestFile = ".cmt-manifest.json"

func (p *SyncPlan) Stats() (int, int, int, int, int, int) {
	hostCount := len(p.HostPlans)
	projectCount := 0
	addCount := 0
	modifyCount := 0
	deleteCount := 0
	unchangedCount := 0

	for _, hostPlan := range p.HostPlans {
		projectCount += len(hostPlan.Projects)

		for _, projectPlan := range hostPlan.Projects {
			for _, filePlan := range projectPlan.Files {
				switch filePlan.Action {
				case ActionAdd:
					addCount++
				case ActionModify:
					modifyCount++
				case ActionDelete:
					deleteCount++
				case ActionUnchanged:
					unchangedCount++
				}
			}
		}
	}

	return hostCount, projectCount, addCount, modifyCount, deleteCount, unchangedCount
}

func (p *SyncPlan) DirStats() (int, int, int) {
	toCreateCount := 0
	toUpdateCount := 0
	existingCount := 0

	for _, hostPlan := range p.HostPlans {
		for _, projectPlan := range hostPlan.Projects {
			for _, directoryPlan := range projectPlan.Dirs {
				switch directoryPlan.Action {
				case ActionAdd:
					toCreateCount++
				case ActionModify:
					toUpdateCount++
				case ActionUnchanged, ActionDelete:
					existingCount++
				}
			}
		}
	}

	return toCreateCount, toUpdateCount, existingCount
}

// PlanHasExistenceUnknown returns true if any directory in the plan has
// ExistenceUnknown (e.g. SSH was unreachable when checking).
func PlanHasExistenceUnknown(plan *SyncPlan) bool {
	for _, hostPlan := range plan.HostPlans {
		for _, projectPlan := range hostPlan.Projects {
			for _, dirPlan := range projectPlan.Dirs {
				if dirPlan.ExistenceUnknown {
					return true
				}
			}
		}
	}

	return false
}

func (p *SyncPlan) ComposeStats() (int, int, int) {
	startCount := 0
	recreateCount := 0
	stopCount := 0

	for _, hostPlan := range p.HostPlans {
		for _, projectPlan := range hostPlan.Projects {
			if projectPlan.Compose == nil {
				continue
			}

			switch projectPlan.Compose.ActionType {
			case ComposeNoChange:
				continue
			case ComposeStartServices:
				startCount += len(projectPlan.Compose.Services)
			case ComposeRecreateServices:
				recreateCount += len(projectPlan.Compose.Services)
			case ComposeStopServices:
				stopCount += len(projectPlan.Compose.Services)
			}
		}
	}

	return startCount, recreateCount, stopCount
}

func (p *SyncPlan) HasChanges() bool {
	hostCount, projectCount, add, mod, del, unchangedCount := p.Stats()
	_ = hostCount
	_ = projectCount
	_ = unchangedCount
	dirCreate, dirUpdate, _ := p.DirStats()

	if add+mod+del+dirCreate+dirUpdate > 0 {
		return true
	}

	for _, hostPlan := range p.HostPlans {
		for _, projectPlan := range hostPlan.Projects {
			if projectPlan.Compose.HasChanges() {
				return true
			}
		}
	}

	return false
}

func (p *SyncPlan) Print(writer io.Writer) {
	style := newOutputStyle(writer)

	if len(p.HostPlans) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("No hosts selected."))

		return
	}

	for _, hostPlan := range p.HostPlans {
		printHostPlan(writer, style, hostPlan)
	}

	hosts, projects, add, mod, del, unch := p.Stats()
	dirCreate, dirUpdate, _ := p.DirStats()

	_, _ = fmt.Fprintf(writer,
		"\n%s %d host(s), %d project(s) — %s to add, %s to modify, %s to delete, %s "+labelUnchanged,
		style.key("Summary:"),
		hosts,
		projects,
		style.success(strconv.Itoa(add)),
		style.warning(strconv.Itoa(mod)),
		style.danger(strconv.Itoa(del)),
		style.muted(strconv.Itoa(unch)))

	if dirCreate > 0 {
		_, _ = fmt.Fprintf(writer, ", %s dir(s) to create", style.success(strconv.Itoa(dirCreate)))
	}

	if dirUpdate > 0 {
		_, _ = fmt.Fprintf(writer, ", %s dir(s) to update", style.warning(strconv.Itoa(dirUpdate)))
	}

	composeStart, composeRecreate, composeStop := p.ComposeStats()
	if composeStart > 0 {
		_, _ = fmt.Fprintf(writer, ", %s service(s) to start", style.success(strconv.Itoa(composeStart)))
	}

	if composeRecreate > 0 {
		_, _ = fmt.Fprintf(writer, ", %s service(s) to recreate", style.warning(strconv.Itoa(composeRecreate)))
	}

	if composeStop > 0 {
		_, _ = fmt.Fprintf(writer, ", %s service(s) to stop", style.danger(strconv.Itoa(composeStop)))
	}

	_, _ = fmt.Fprintln(writer)

	for _, hostPlan := range p.HostPlans {
		printHostSummaryTable(writer, style, hostPlan)
	}

	_, _ = fmt.Fprintln(writer)
}

func printHostPlan(writer io.Writer, style outputStyle, hostPlan HostPlan) {
	hostLine := fmt.Sprintf(
		"=== Host: %s (%s@%s:%d) ===",
		hostPlan.Host.Name,
		hostPlan.Host.User,
		hostPlan.Host.Host,
		hostPlan.Host.Port,
	)
	_, _ = fmt.Fprintf(writer, "\n%s\n", style.hostHeader(hostLine))

	if len(hostPlan.Projects) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("  (no projects)"))

		return
	}

	for _, projectPlan := range hostPlan.Projects {
		if projectPlan.HasChanges() {
			printProjectPlan(writer, style, projectPlan)
		} else {
			printProjectPlanCollapsed(writer, style, projectPlan)
		}
	}
}

const (
	summaryTableColProject       = 20
	summaryTableColStatus        = 10
	summaryTableColComposeAction = 18
	summaryTableSeparator        = "----------------------------------------------------------"
)

func printHostSummaryTable(writer io.Writer, style outputStyle, hostPlan HostPlan) {
	_, _ = fmt.Fprintf(writer, "\n%s\n", style.hostHeader("Host: "+hostPlan.Host.Name))
	_, _ = fmt.Fprintln(writer, summaryTableSeparator)
	_, _ = fmt.Fprintf(writer, "%-*s %-*s %-*s %s\n",
		summaryTableColProject, "PROJECT",
		summaryTableColStatus, "STATUS",
		summaryTableColComposeAction, "COMPOSE ACTION",
		"RESOURCES")
	_, _ = fmt.Fprintln(writer, summaryTableSeparator)

	for _, pp := range hostPlan.Projects {
		status := labelUnchanged
		if pp.HasChanges() {
			status = "changed"
		}

		composeAction := formatComposeActionSummary(pp.Compose)
		resources := formatProjectResourcesSummary(pp)
		_, _ = fmt.Fprintf(writer, "%-*s %-*s %-*s %s\n",
			summaryTableColProject, pp.ProjectName,
			summaryTableColStatus, status,
			summaryTableColComposeAction, composeAction,
			resources)
	}
}

func formatComposeActionSummary(compose *ComposePlan) string {
	if compose == nil || len(compose.Services) == 0 {
		return "-"
	}

	n := len(compose.Services)
	switch compose.ActionType {
	case ComposeNoChange:
		return "-"
	case ComposeStartServices:
		return fmt.Sprintf("start (%d)", n)
	case ComposeRecreateServices:
		return fmt.Sprintf("recreate (%d)", n)
	case ComposeStopServices:
		return fmt.Sprintf("stop (%d)", n)
	default:
		return "-"
	}
}

func formatProjectResourcesSummary(pp ProjectPlan) string {
	parts := fileResourceParts(pp.Files)

	parts = append(parts, dirResourceParts(pp.Dirs)...)
	if pp.Compose != nil && pp.Compose.ActionType != ComposeNoChange && len(pp.Compose.Services) > 0 {
		parts = append(parts, fmt.Sprintf("%d svc", len(pp.Compose.Services)))
	}

	if len(parts) == 0 {
		return "-"
	}

	return strings.Join(parts, "; ")
}

func fileResourceParts(files []FilePlan) []string {
	add, mod, del := 0, 0, 0

	for _, f := range files {
		switch f.Action {
		case ActionUnchanged:
			// no op
		case ActionAdd:
			add++
		case ActionModify:
			mod++
		case ActionDelete:
			del++
		}
	}

	if add+mod+del == 0 {
		return nil
	}

	return []string{fmt.Sprintf("%d add, %d mod, %d del files", add, mod, del)}
}

func dirResourceParts(dirs []DirPlan) []string {
	dirAdd, dirMod := 0, 0

	for _, d := range dirs {
		switch d.Action {
		case ActionUnchanged, ActionDelete:
			// no op
		case ActionAdd:
			dirAdd++
		case ActionModify:
			dirMod++
		}
	}

	if dirAdd+dirMod == 0 {
		return nil
	}

	return []string{fmt.Sprintf("%d create, %d update dirs", dirAdd, dirMod)}
}

func printProjectPlan(writer io.Writer, style outputStyle, projectPlan ProjectPlan) {
	_, _ = fmt.Fprintf(writer, "\n  %s %s\n", style.key("Project:"), style.projectName(projectPlan.ProjectName))
	_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("Remote:"), projectPlan.RemoteDir)

	if projectPlan.PostSyncCommand != "" {
		_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("Post-sync:"), projectPlan.PostSyncCommand)
	}

	if projectPlan.ComposeAction != "" {
		_, _ = fmt.Fprintf(writer, "    %s %s\n", style.key("Compose:"), projectPlan.ComposeAction)
	}

	_, _ = fmt.Fprintln(writer)
	printComposePlan(writer, style, projectPlan.Compose)
	printProjectDirPlans(writer, style, projectPlan.Dirs)

	if len(projectPlan.Files) == 0 && len(projectPlan.Dirs) == 0 {
		_, _ = fmt.Fprintln(writer, style.muted("    (no files or dirs)"))

		return
	}

	for _, filePlan := range projectPlan.Files {
		_, _ = fmt.Fprintf(
			writer,
			"    %s %s (%s)\n",
			style.actionSymbol(filePlan.Action),
			filePlan.RelativePath,
			filePlanLabel(filePlan),
		)

		printFileDiff(writer, style, filePlan.Diff)
	}
}

func printProjectPlanCollapsed(writer io.Writer, style outputStyle, projectPlan ProjectPlan) {
	_, _ = fmt.Fprintf(writer, "\n  %s %s %s\n",
		style.actionSymbol(ActionUnchanged),
		style.projectName(projectPlan.ProjectName),
		style.muted("(no changes)"))
}

func printComposePlan(writer io.Writer, style outputStyle, compose *ComposePlan) {
	if compose == nil || compose.ActionType == ComposeNoChange || len(compose.Services) == 0 {
		return
	}

	_, _ = fmt.Fprintln(writer, "    "+style.key("Compose services:"))

	for _, svc := range compose.Services {
		switch compose.ActionType {
		case ComposeNoChange:
			continue
		case ComposeStartServices:
			_, _ = fmt.Fprintf(writer, "      %s %s %s\n",
				style.actionSymbol(ActionAdd), svc, style.success("(start)"))
		case ComposeRecreateServices:
			_, _ = fmt.Fprintf(writer, "      %s %s %s\n",
				style.actionSymbol(ActionModify), svc, style.warning("(recreate)"))
		case ComposeStopServices:
			_, _ = fmt.Fprintf(writer, "      %s %s %s\n",
				style.actionSymbol(ActionDelete), svc, style.danger("(stop)"))
		}
	}

	_, _ = fmt.Fprintln(writer)
}

func printProjectDirPlans(writer io.Writer, style outputStyle, dirPlans []DirPlan) {
	if len(dirPlans) == 0 {
		return
	}

	_, _ = fmt.Fprintln(writer, "    "+style.key("Dirs:"))

	for _, directoryPlan := range dirPlans {
		var statusText string

		switch directoryPlan.Action {
		case ActionAdd:
			if directoryPlan.ExistenceUnknown {
				statusText = style.warning("(unknown)")
			} else {
				statusText = style.success("(create)")
			}
		case ActionModify:
			statusText = style.warning("(update)")
		case ActionUnchanged:
			statusText = style.muted("(exists)")
		case ActionDelete:
			statusText = style.danger("(delete)")
		}

		extra := formatDirPlanMeta(directoryPlan)

		_, _ = fmt.Fprintf(
			writer,
			"      %s %s/ %s%s\n",
			style.actionSymbol(directoryPlan.Action),
			directoryPlan.RelativePath,
			statusText,
			extra,
		)
	}

	_, _ = fmt.Fprintln(writer)
}

func formatDirPlanMeta(dirPlan DirPlan) string {
	var parts []string

	switch dirPlan.Action {
	case ActionAdd:
		if dirPlan.Permission != "" {
			parts = append(parts, "mode="+dirPlan.Permission)
		}

		if dirPlan.Owner != "" || dirPlan.Group != "" {
			parts = append(parts, "owner="+formatOwnership(dirPlan.Owner, dirPlan.Group))
		}
	case ActionModify:
		if dirPlan.NeedsPermChange {
			parts = append(parts, fmt.Sprintf("mode: %s\u2192%s", dirPlan.ActualPermission, dirPlan.Permission))
		}

		if dirPlan.NeedsOwnerChange {
			parts = append(parts, fmt.Sprintf("owner: %s\u2192%s",
				formatOwnership(dirPlan.ActualOwner, dirPlan.ActualGroup),
				formatOwnership(dirPlan.Owner, dirPlan.Group)))
		}
	case ActionUnchanged, ActionDelete:
	}

	if len(parts) == 0 {
		return ""
	}

	return " [" + strings.Join(parts, ", ") + "]"
}

func formatOwnership(owner, group string) string {
	if group != "" {
		return owner + ":" + group
	}

	return owner
}

func filePlanLabel(filePlan FilePlan) string {
	switch filePlan.Action {
	case ActionAdd:
		return "new, " + humanSize(len(filePlan.LocalData))
	case ActionModify:
		return "modified"
	case ActionDelete:
		return "delete"
	case ActionUnchanged:
		return labelUnchanged
	default:
		return "unknown"
	}
}

func printFileDiff(writer io.Writer, style outputStyle, diff string) {
	if diff == "" {
		return
	}

	for line := range strings.SplitSeq(diff, "\n") {
		if line == "" {
			continue
		}

		_, _ = fmt.Fprintf(writer, "        %s\n", style.diffLine(line))
	}
}

func BuildPlan(cfg *config.CmtConfig, hostFilter, projectFilter []string) (*SyncPlan, error) {
	var dependencies PlanDependencies

	return BuildPlanWithDeps(cfg, hostFilter, projectFilter, dependencies)
}

func BuildPlanWithDeps(
	cfg *config.CmtConfig,
	hostFilter, projectFilter []string,
	deps PlanDependencies,
) (*SyncPlan, error) {
	clientFactory, sshResolver, localRunner := resolvePlanDependencies(deps)
	progress := resolvePlanProgress(deps.ProgressWriter)

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

	progress.planStart(len(hosts), len(projects))

	plan := new(SyncPlan)
	plan.HostPlans = nil

	for hostIdx, host := range hosts {
		progress.hostStart(hostIdx+1, len(hosts), host.Name)

		hostPlan, err := buildHostPlanForTarget(cfg, host, projects, clientFactory, sshResolver, localRunner, progress)
		if err != nil {
			return nil, err
		}

		progress.hostDone(hostIdx+1, len(hosts), host.Name)

		plan.HostPlans = append(plan.HostPlans, *hostPlan)
	}

	progress.planDone()

	return plan, nil
}

type planProgress struct {
	mu     *sync.Mutex
	writer io.Writer
	style  outputStyle
}

func resolvePlanProgress(writer io.Writer) planProgress {
	if writer == nil {
		return planProgress{
			mu:     &sync.Mutex{},
			writer: io.Discard,
			style: outputStyle{
				enabled: false,
			},
		}
	}

	return planProgress{mu: &sync.Mutex{}, writer: writer, style: newOutputStyle(writer)}
}

func (p planProgress) planStart(hostCount, projectCount int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintf(p.writer, "%s %d host(s), %d project(s)\n",
		p.style.key("Planning:"), hostCount, projectCount)
}

func (p planProgress) hostStart(index, total int, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintf(p.writer, "%s %s %s\n",
		p.style.key(fmt.Sprintf("Planning host %d/%d:", index, total)),
		p.style.projectName(name),
		p.style.muted("connecting..."))
}

func (p planProgress) hostDone(index, total int, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintf(p.writer, "%s %s %s\n",
		p.style.key(fmt.Sprintf("Planning host %d/%d:", index, total)),
		p.style.projectName(name),
		p.style.success("done"))
}

func (p planProgress) projectStart(index, total int, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintf(p.writer, "  %s %s\n",
		p.style.muted(fmt.Sprintf("project %d/%d:", index, total)),
		p.style.projectName(name))
}

func (p planProgress) projectDone(index, total int, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintf(p.writer, "  %s %s %s\n",
		p.style.muted(fmt.Sprintf("project %d/%d:", index, total)),
		p.style.projectName(name),
		p.style.success("done"))
}

func (p planProgress) planDone() {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, _ = fmt.Fprintln(p.writer, p.style.success("Plan complete."))
}

func resolvePlanDependencies(
	deps PlanDependencies,
) (remote.ClientFactory, config.SSHConfigResolver, LocalCommandRunner) {
	clientFactory := deps.ClientFactory
	if clientFactory == nil {
		defaultFactory := new(remote.DefaultClientFactory)
		defaultFactory.Runner = nil
		clientFactory = *defaultFactory
	}

	sshResolver := deps.SSHResolver
	if sshResolver == nil {
		defaultResolver := new(config.DefaultSSHConfigResolver)
		defaultResolver.Runner = nil
		sshResolver = *defaultResolver
	}

	localRunner := deps.LocalRunner
	if localRunner == nil {
		localRunner = ExecLocalCommandRunner{}
	}

	return clientFactory, sshResolver, localRunner
}

func buildHostPlanForTarget(
	cfg *config.CmtConfig,
	host config.HostEntry,
	projects []string,
	clientFactory remote.ClientFactory,
	sshResolver config.SSHConfigResolver,
	localRunner LocalCommandRunner,
	progress planProgress,
) (*HostPlan, error) {
	hostCfg, found, err := loadHostConfig(cfg.BasePath, host.Name)
	if err != nil {
		return nil, fmt.Errorf("loading host config for %s: %w", host.Name, err)
	}

	if !found {
		hostCfg = nil
	}

	err = resolveHostSSHConfig(cfg.BasePath, &host, hostCfg, sshResolver)
	if err != nil {
		return nil, fmt.Errorf("resolving SSH config for %s: %w", host.Name, err)
	}

	client, err := clientFactory.NewClient(host)
	if err != nil {
		return nil, fmt.Errorf("connecting to host %s: %w", host.Name, err)
	}

	defer func() {
		_ = client.Close()
	}()

	hostPlan, err := buildHostPlan(cfg, host, hostCfg, projects, client, localRunner, progress)
	if err != nil {
		return nil, err
	}

	return hostPlan, nil
}

func loadHostConfig(basePath, hostName string) (*config.HostConfig, bool, error) {
	hostCfg, err := config.LoadHostConfig(basePath, hostName)
	if errors.Is(err, config.ErrHostConfigNotFound) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	return hostCfg, true, nil
}

func resolveHostSSHConfig(
	basePath string,
	host *config.HostEntry,
	hostCfg *config.HostConfig,
	sshResolver config.SSHConfigResolver,
) error {
	sshConfigPath := ""
	if hostCfg != nil {
		sshConfigPath = hostCfg.SSHConfig
	}

	hostDir := filepath.Join(basePath, "hosts", host.Name)

	return sshResolver.Resolve(host, sshConfigPath, hostDir)
}

type projectResult struct {
	plan ProjectPlan
	err  error
}

func buildHostPlan(
	cfg *config.CmtConfig,
	host config.HostEntry,
	hostCfg *config.HostConfig,
	projects []string,
	client remote.RemoteClient,
	localRunner LocalCommandRunner,
	progress planProgress,
) (*HostPlan, error) {
	if hostCfg != nil && len(hostCfg.Projects) > 0 {
		filtered := make([]string, 0, len(projects))
		for _, p := range projects {
			if _, ok := hostCfg.Projects[p]; ok {
				filtered = append(filtered, p)
			}
		}
		projects = filtered
	}

	hostPlan := &HostPlan{
		Host:     host,
		Projects: make([]ProjectPlan, len(projects)),
	}

	results := make([]projectResult, len(projects))

	var waitGroup sync.WaitGroup

	for i, project := range projects {
		waitGroup.Add(1)

		go func(idx int, proj string) {
			defer waitGroup.Done()

			progress.projectStart(idx+1, len(projects), proj)

			plan, err := buildProjectPlanForHost(cfg, host, hostCfg, proj, client, localRunner)
			results[idx] = projectResult{plan: plan, err: err}

			if err == nil {
				progress.projectDone(idx+1, len(projects), proj)
			}
		}(i, project)
	}

	waitGroup.Wait()

	for i := range projects {
		if results[i].err != nil {
			return nil, results[i].err
		}

		hostPlan.Projects[i] = results[i].plan
	}

	return hostPlan, nil
}

func buildProjectPlanForHost(
	cfg *config.CmtConfig,
	host config.HostEntry,
	hostCfg *config.HostConfig,
	project string,
	client remote.RemoteClient,
	localRunner LocalCommandRunner,
) (ProjectPlan, error) {
	resolved := config.ResolveProjectConfig(cfg.Defaults, hostCfg, project)
	if resolved.RemotePath == "" {
		return ProjectPlan{}, fmt.Errorf("%w for host %q, project %q", errRemotePathNotSet, host.Name, project)
	}

	remoteDir := path.Join(resolved.RemotePath, project)
	dirPlans := buildDirPlans(resolved.Dirs, remoteDir, client)

	templateVars, err := LoadTemplateVars(cfg.BasePath, host.Name, project, resolved.TemplateVarSources)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("loading template vars for %s/%s: %w", host.Name, project, err)
	}

	localFiles, err := CollectLocalFiles(cfg.BasePath, host.Name, project)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("collecting files for %s/%s: %w", host.Name, project, err)
	}

	manifest := readManifest(client, remoteDir)

	filePlans, err := buildFilePlans(localFiles, remoteDir, manifest, client, templateVars)
	if err != nil {
		return ProjectPlan{}, fmt.Errorf("building file plan for %s/%s: %w", host.Name, project, err)
	}

	err = validateComposeConfigForPlan(host.Name, project, filePlans, localRunner)
	if err != nil {
		return ProjectPlan{}, err
	}

	hasFileChanges := projectFilesOrDirsChanged(filePlans, dirPlans)
	composePlan := buildComposePlan(resolved.ComposeAction, remoteDir, client, hasFileChanges)

	return ProjectPlan{
		ProjectName:     project,
		RemoteDir:       remoteDir,
		PostSyncCommand: resolved.PostSyncCommand,
		ComposeAction:   resolved.ComposeAction,
		RemoveOrphans:   resolved.RemoveOrphans,
		Compose:         composePlan,
		Dirs:            dirPlans,
		Files:           filePlans,
	}, nil
}

func validateComposeConfigForPlan(
	hostName string,
	projectName string,
	filePlans []FilePlan,
	localRunner LocalCommandRunner,
) error {
	filesToValidate := collectComposeValidationFiles(filePlans)

	if _, hasCompose := filesToValidate["compose.yml"]; !hasCompose {
		return nil
	}

	tempDir, err := os.MkdirTemp("", "cmt-compose-validate-*")
	if err != nil {
		return fmt.Errorf("creating temporary directory for compose validation: %w", err)
	}

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	err = writeComposeValidationFiles(tempDir, filesToValidate)
	if err != nil {
		return err
	}

	args := buildComposeConfigArgs(filesToValidate)

	output, runErr := localRunner.Run("docker", args, tempDir)
	if runErr != nil {
		return fmt.Errorf(
			"validating docker compose config for %s/%s failed: %w\n%s",
			hostName,
			projectName,
			runErr,
			strings.TrimSpace(output),
		)
	}

	return nil
}

const (
	composeValidationDirPerm  fs.FileMode = 0o750
	composeValidationFilePerm fs.FileMode = 0o600
)

func collectComposeValidationFiles(filePlans []FilePlan) map[string][]byte {
	filesToValidate := make(map[string][]byte)

	for _, plan := range filePlans {
		if plan.Action == ActionDelete || len(plan.LocalData) == 0 {
			continue
		}

		filesToValidate[plan.RelativePath] = plan.LocalData
	}

	return filesToValidate
}

func writeComposeValidationFiles(baseDir string, files map[string][]byte) error {
	for relPath, data := range files {
		targetPath := filepath.Join(baseDir, relPath)

		targetDir := filepath.Dir(targetPath)

		mkdirErr := os.MkdirAll(targetDir, composeValidationDirPerm)
		if mkdirErr != nil {
			return fmt.Errorf("preparing compose validation file %s: %w", relPath, mkdirErr)
		}

		writeErr := os.WriteFile(targetPath, data, composeValidationFilePerm)
		if writeErr != nil {
			return fmt.Errorf("writing compose validation file %s: %w", relPath, writeErr)
		}
	}

	return nil
}

func buildComposeConfigArgs(filesToValidate map[string][]byte) []string {
	args := []string{"compose", "-f", "compose.yml"}
	if _, hasOverride := filesToValidate["compose.override.yml"]; hasOverride {
		args = append(args, "-f", "compose.override.yml")
	}

	return append(args, "config")
}

func buildDirPlans(directories []config.DirConfig, remoteDir string, client remote.RemoteClient) []DirPlan {
	dirPlans := make([]DirPlan, 0, len(directories))

	for _, directory := range directories {
		absDir := path.Join(remoteDir, directory.Path)
		_, statErr := client.Stat(absDir)
		exists := statErr == nil
		existenceUnknown := errors.Is(statErr, remote.ErrExistenceUnknown)

		plan := DirPlan{
			RelativePath:     directory.Path,
			RemotePath:       absDir,
			Exists:           exists,
			ExistenceUnknown: existenceUnknown,
			Action:           ActionUnchanged,
			Permission:       directory.Permission,
			Owner:            directory.Owner,
			Group:            directory.Group,
			Become:           directory.Become,
			BecomeUser:       directory.BecomeUser,
			Recursive:        directory.Recursive,
			ActualPermission: "",
			ActualOwner:      "",
			ActualGroup:      "",
			NeedsPermChange:  false,
			NeedsOwnerChange: false,
		}

		switch {
		case existenceUnknown:
			plan.Action = ActionAdd
			plan.NeedsPermChange = directory.Permission != ""
			plan.NeedsOwnerChange = directory.Owner != "" || directory.Group != ""
		case !exists:
			plan.Action = ActionAdd
			plan.NeedsPermChange = directory.Permission != ""
			plan.NeedsOwnerChange = directory.Owner != "" || directory.Group != ""
		default:
			computeDirDrift(&plan, client)
		}

		dirPlans = append(dirPlans, plan)
	}

	return dirPlans
}

func computeDirDrift(plan *DirPlan, client remote.RemoteClient) {
	if !dirPlanHasDesiredMetadata(plan) {
		plan.Action = ActionUnchanged

		return
	}

	meta, err := client.StatDirMetadata(plan.RemotePath)
	if err != nil {
		markDirPlanAsNeedsMetadataUpdate(plan)

		return
	}

	applyActualDirMetadata(plan, meta)
	computeDirPlanMetadataDrift(plan, meta)
	setDirPlanActionFromMetadataDrift(plan)
}

func dirPlanHasDesiredMetadata(plan *DirPlan) bool {
	return plan.Permission != "" || plan.Owner != "" || plan.Group != ""
}

func markDirPlanAsNeedsMetadataUpdate(plan *DirPlan) {
	plan.Action = ActionModify
	plan.NeedsPermChange = plan.Permission != ""
	plan.NeedsOwnerChange = plan.Owner != "" || plan.Group != ""
}

func applyActualDirMetadata(plan *DirPlan, meta *remote.DirMetadata) {
	plan.ActualPermission = meta.Permission
	plan.ActualOwner = meta.Owner
	plan.ActualGroup = meta.Group
}

func computeDirPlanMetadataDrift(plan *DirPlan, meta *remote.DirMetadata) {
	if plan.Permission != "" && !permissionsMatch(plan.Permission, meta.Permission) {
		plan.NeedsPermChange = true
	}

	if plan.Owner != "" && !ownershipMatches(plan.Owner, meta.Owner, meta.OwnerID) {
		plan.NeedsOwnerChange = true
	}

	if plan.Group != "" && !ownershipMatches(plan.Group, meta.Group, meta.GroupID) {
		plan.NeedsOwnerChange = true
	}
}

func ownershipMatches(desired, actualName, actualID string) bool {
	return desired == actualName || desired == actualID
}

func setDirPlanActionFromMetadataDrift(plan *DirPlan) {
	if plan.NeedsPermChange || plan.NeedsOwnerChange {
		plan.Action = ActionModify
	} else {
		plan.Action = ActionUnchanged
	}
}

func permissionsMatch(desired, actual string) bool {
	dVal, dErr := strconv.ParseUint(desired, 8, 32)
	aVal, aErr := strconv.ParseUint(actual, 8, 32)

	if dErr != nil || aErr != nil {
		return desired == actual
	}

	return dVal == aVal
}

func buildComposePlan(action string, remoteDir string, client remote.RemoteClient, hasFileChanges bool) *ComposePlan {
	plan := &ComposePlan{
		DesiredAction: action,
		ActionType:    ComposeNoChange,
		Services:      nil,
	}

	switch action {
	case config.ComposeActionUp:
		defined := queryComposeServices(client, remoteDir)
		running := queryRunningServices(client, remoteDir)

		if hasFileChanges && len(defined) > 0 {
			plan.ActionType = ComposeRecreateServices
			plan.Services = defined
		} else {
			stopped := diffServices(defined, running)
			if len(stopped) > 0 {
				plan.ActionType = ComposeStartServices
				plan.Services = stopped
			}
		}
	case config.ComposeActionDown:
		running := queryRunningServices(client, remoteDir)
		if len(running) > 0 {
			plan.ActionType = ComposeStopServices
			plan.Services = running
		}
	case config.ComposeActionIgnore:
		// Explicitly ignore runtime up/down state drift for this project.
		return plan
	}

	return plan
}

func queryComposeServices(client remote.RemoteClient, remoteDir string) []string {
	output, err := client.RunCommand(remoteDir, "docker compose config --services 2>/dev/null")
	if err != nil {
		return nil
	}

	return parseServiceLines(output)
}

func queryRunningServices(client remote.RemoteClient, remoteDir string) []string {
	output, err := client.RunCommand(remoteDir, "docker compose ps --services --filter status=running 2>/dev/null")
	if err != nil {
		return nil
	}

	return parseServiceLines(output)
}

func parseServiceLines(output string) []string {
	var services []string

	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			services = append(services, line)
		}
	}

	sort.Strings(services)

	return services
}

func diffServices(all, running []string) []string {
	runningSet := make(map[string]bool, len(running))
	for _, s := range running {
		runningSet[s] = true
	}

	var stopped []string

	for _, s := range all {
		if !runningSet[s] {
			stopped = append(stopped, s)
		}
	}

	return stopped
}

func projectFilesOrDirsChanged(filePlans []FilePlan, dirPlans []DirPlan) bool {
	for _, fp := range filePlans {
		if fp.Action != ActionUnchanged {
			return true
		}
	}

	for _, dp := range dirPlans {
		if dp.Action != ActionUnchanged {
			return true
		}
	}

	return false
}

func buildFilePlans(
	localFiles map[string]string,
	remoteDir string,
	manifest *Manifest,
	client remote.RemoteClient,
	templateVars map[string]any,
) ([]FilePlan, error) {
	plans := make([]FilePlan, 0, len(localFiles))
	localSet := make(map[string]bool, len(localFiles))

	for relPath, localPath := range localFiles {
		localSet[relPath] = true

		filePlan, err := buildLocalFilePlan(
			relPath,
			localPath,
			remoteDir,
			client,
			templateVars,
			maskHintsFromManifest(manifest, relPath),
		)
		if err != nil {
			return nil, err
		}

		plans = append(plans, filePlan)
	}

	plans = append(plans, buildDeleteFilePlans(manifest, localSet, remoteDir, client)...)

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].RelativePath < plans[j].RelativePath
	})

	return plans, nil
}

func buildLocalFilePlan(
	relPath string,
	localPath string,
	remoteDir string,
	client remote.RemoteClient,
	templateVars map[string]any,
	manifestMaskHints []MaskHint,
) (FilePlan, error) {
	cleanLocalPath := filepath.Clean(localPath)

	rawData, err := os.ReadFile(cleanLocalPath)
	if err != nil {
		return FilePlan{}, fmt.Errorf("reading %s: %w", cleanLocalPath, err)
	}

	localData, err := RenderTemplate(rawData, templateVars)
	if err != nil {
		return FilePlan{}, fmt.Errorf("rendering template %s: %w", cleanLocalPath, err)
	}

	patterns := buildDiffMaskPatterns(rawData, localData, templateVars)
	allPatterns := mergeMaskPatterns(patterns, patternsFromMaskHints(manifestMaskHints))

	remotePath := path.Join(remoteDir, relPath)
	remoteData, readErr := client.ReadFile(remotePath)

	filePlan := FilePlan{
		RelativePath: relPath,
		LocalPath:    localPath,
		RemotePath:   remotePath,
		Action:       ActionUnchanged,
		LocalData:    localData,
		RemoteData:   nil,
		Diff:         "",
		MaskHints:    maskHintsFromPatterns(allPatterns),
	}

	if readErr == nil {
		filePlan.RemoteData = remoteData
		if bytes.Equal(localData, remoteData) {
			return filePlan, nil
		}

		filePlan.Action = ActionModify
		if !isBinary(localData) && !isBinary(remoteData) {
			filePlan.Diff = maskDiffWithPatterns(
				computeDiff(relPath, remoteData, localData),
				allPatterns,
			)
		}

		return filePlan, nil
	}

	filePlan.Action = ActionAdd

	return filePlan, nil
}

func buildDeleteFilePlans(
	manifest *Manifest,
	localSet map[string]bool,
	remoteDir string,
	client remote.RemoteClient,
) []FilePlan {
	if manifest == nil {
		return nil
	}

	deletePlans := make([]FilePlan, 0)

	for _, managedFile := range manifest.ManagedFiles {
		if managedFile == manifestFile || localSet[managedFile] {
			continue
		}

		remotePath := path.Join(remoteDir, managedFile)
		remoteData, _ := client.ReadFile(remotePath)
		deletePlans = append(deletePlans, FilePlan{
			RelativePath: managedFile,
			LocalPath:    "",
			RemotePath:   remotePath,
			Action:       ActionDelete,
			LocalData:    nil,
			RemoteData:   remoteData,
			Diff:         "",
			MaskHints:    nil,
		})
	}

	return deletePlans
}

func CollectLocalFiles(basePath, hostName, projectName string) (map[string]string, error) {
	files := make(map[string]string)

	projectDir := filepath.Join(basePath, "projects", projectName)
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)

	if composePath := filepath.Join(projectDir, "compose.yml"); fileExists(composePath) {
		files["compose.yml"] = composePath
	}

	err := walkFiles(filepath.Join(projectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	if overridePath := filepath.Join(hostProjectDir, "compose.override.yml"); fileExists(overridePath) {
		files["compose.override.yml"] = overridePath
	}

	err = walkFiles(filepath.Join(hostProjectDir, "files"), files)
	if err != nil {
		return nil, err
	}

	return files, nil
}

func readManifest(client remote.RemoteClient, remoteDir string) *Manifest {
	data, err := client.ReadFile(path.Join(remoteDir, manifestFile))
	if err != nil {
		return nil
	}

	var manifest Manifest
	if json.Unmarshal(data, &manifest) != nil {
		return nil
	}

	return &manifest
}

func BuildManifest(localFiles map[string]string) Manifest {
	return BuildManifestWithMaskHints(localFiles, nil)
}

func BuildManifestWithMaskHints(localFiles map[string]string, maskHints map[string][]MaskHint) Manifest {
	var manifest Manifest
	for rel := range localFiles {
		manifest.ManagedFiles = append(manifest.ManagedFiles, rel)
	}

	sort.Strings(manifest.ManagedFiles)

	if len(maskHints) > 0 {
		manifest.MaskHints = make(map[string][]MaskHint, len(maskHints))
		for relPath, hints := range maskHints {
			if !containsString(manifest.ManagedFiles, relPath) || len(hints) == 0 {
				continue
			}

			manifest.MaskHints[relPath] = append([]MaskHint(nil), hints...)
		}

		if len(manifest.MaskHints) == 0 {
			manifest.MaskHints = nil
		}
	}

	return manifest
}

func containsString(values []string, target string) bool {
	return slices.Contains(values, target)
}

func maskHintsFromManifest(manifest *Manifest, relPath string) []MaskHint {
	if manifest == nil || len(manifest.MaskHints) == 0 {
		return nil
	}

	hints, ok := manifest.MaskHints[relPath]
	if !ok {
		return nil
	}

	return append([]MaskHint(nil), hints...)
}

func computeDiff(name string, remote, local []byte) string {
	diff := new(difflib.UnifiedDiff)
	diff.A = difflib.SplitLines(string(remote))
	diff.B = difflib.SplitLines(string(local))
	diff.FromFile = name + " (remote)"
	diff.ToFile = name + " (local)"
	diff.FromDate = ""
	diff.ToDate = ""
	diff.Context = diffContextLines
	diff.Eol = ""
	text, _ := difflib.GetUnifiedDiffString(*diff)

	return text
}

func isBinary(data []byte) bool {
	check := data
	if len(check) > binaryProbeBytes {
		check = check[:binaryProbeBytes]
	}

	return bytes.ContainsRune(check, 0)
}

func fileExists(p string) bool {
	info, err := os.Stat(p)

	return err == nil && !info.IsDir()
}

func walkFiles(dir string, out map[string]string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("stat %s: %w", dir, err)
	}

	if !info.IsDir() {
		return nil
	}

	return filepath.WalkDir(dir, func(pathValue string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, _ := filepath.Rel(dir, pathValue)
		out[rel] = pathValue

		return nil
	})
}

const (
	kiloBytes        = 1024
	megaBytes        = 1024 * kiloBytes
	diffContextLines = 3
	binaryProbeBytes = 8192
)

func humanSize(byteCount int) string {
	switch {
	case byteCount >= megaBytes:
		return fmt.Sprintf("%.1f MB", float64(byteCount)/float64(megaBytes))
	case byteCount >= kiloBytes:
		return fmt.Sprintf("%.1f KB", float64(byteCount)/float64(kiloBytes))
	default:
		return fmt.Sprintf("%d B", byteCount)
	}
}

const maskPlaceholder = "***"

type maskPattern struct {
	prefix string
	suffix string
}

func patternsFromMaskHints(hints []MaskHint) []maskPattern {
	if len(hints) == 0 {
		return nil
	}

	patterns := make([]maskPattern, 0, len(hints))

	for _, hint := range hints {
		if hint.Prefix == "" && hint.Suffix == "" {
			continue
		}

		patterns = append(patterns, maskPattern{
			prefix: hint.Prefix,
			suffix: hint.Suffix,
		})
	}

	return patterns
}

func maskHintsFromPatterns(patterns []maskPattern) []MaskHint {
	if len(patterns) == 0 {
		return nil
	}

	hints := make([]MaskHint, 0, len(patterns))
	for _, pattern := range patterns {
		hints = append(hints, MaskHint{
			Prefix: pattern.prefix,
			Suffix: pattern.suffix,
		})
	}

	return hints
}

func mergeMaskPatterns(primary []maskPattern, secondary []maskPattern) []maskPattern {
	if len(primary) == 0 {
		return append([]maskPattern(nil), secondary...)
	}

	merged := append([]maskPattern(nil), primary...)
	seen := make(map[string]struct{}, len(primary)+len(secondary))

	for _, pattern := range primary {
		seen[pattern.prefix+"\x00"+pattern.suffix] = struct{}{}
	}

	for _, pattern := range secondary {
		key := pattern.prefix + "\x00" + pattern.suffix
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}

		merged = append(merged, pattern)
	}

	return merged
}

func buildDiffMaskPatterns(
	rawData []byte,
	renderedData []byte,
	vars map[string]any,
) []maskPattern {
	if len(vars) == 0 {
		return nil
	}

	maskedVars := buildMaskedVars(vars)

	maskedData, err := RenderTemplate(rawData, maskedVars)
	if err != nil {
		return nil
	}

	return extractMaskPatterns(renderedData, maskedData)
}

func buildMaskedVars(vars map[string]any) map[string]any {
	masked := make(map[string]any, len(vars))

	for k := range vars {
		masked[k] = maskPlaceholder
	}

	return masked
}

func extractMaskPatterns(rendered, masked []byte) []maskPattern {
	renderedLines := strings.Split(string(rendered), "\n")
	maskedLines := strings.Split(string(masked), "\n")

	limit := min(len(renderedLines), len(maskedLines))
	patterns := make([]maskPattern, 0, limit)

	for i := range limit {
		if renderedLines[i] == maskedLines[i] {
			continue
		}

		prefix := longestCommonPrefix(renderedLines[i], maskedLines[i])

		renderedRest := renderedLines[i][len(prefix):]
		maskedRest := maskedLines[i][len(prefix):]
		suffix := longestCommonSuffix(renderedRest, maskedRest)

		patterns = append(patterns, maskPattern{
			prefix: prefix,
			suffix: suffix,
		})
	}

	return patterns
}

func maskDiffWithPatterns(diff string, patterns []maskPattern) string {
	if len(patterns) == 0 {
		return diff
	}

	var builder strings.Builder

	for line := range strings.SplitSeq(diff, "\n") {
		builder.WriteString(applyMaskToLine(line, patterns))
		builder.WriteByte('\n')
	}

	result := builder.String()
	if len(result) > 0 && (len(diff) == 0 || diff[len(diff)-1] != '\n') {
		result = result[:len(result)-1]
	}

	return result
}

func applyMaskToLine(line string, patterns []maskPattern) string {
	if !isDiffContentLine(line) {
		return line
	}

	diffPrefix := string(line[0])
	content := line[1:]

	for _, pat := range patterns {
		if matchesPattern(content, pat) {
			return diffPrefix + pat.prefix + maskPlaceholder + pat.suffix
		}
	}

	return line
}

func isDiffContentLine(line string) bool {
	if len(line) == 0 {
		return false
	}

	if strings.HasPrefix(line, "---") ||
		strings.HasPrefix(line, "+++") ||
		strings.HasPrefix(line, "@@") {
		return false
	}

	return line[0] == '+' || line[0] == '-' || line[0] == ' '
}

func matchesPattern(content string, pat maskPattern) bool {
	if !strings.HasPrefix(content, pat.prefix) {
		return false
	}

	if pat.suffix != "" && !strings.HasSuffix(content, pat.suffix) {
		return false
	}

	return true
}

func longestCommonPrefix(first, second string) string {
	i := 0

	for i < len(first) && i < len(second) && first[i] == second[i] {
		i++
	}

	return first[:i]
}

func longestCommonSuffix(first, second string) string {
	i := 0

	for i < len(first) && i < len(second) && first[len(first)-1-i] == second[len(second)-1-i] {
		i++
	}

	if i == 0 {
		return ""
	}

	return first[len(first)-i:]
}
