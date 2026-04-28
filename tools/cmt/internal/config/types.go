package config

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/invopop/jsonschema"
	orderedmap "github.com/wk8/go-ordered-map/v2"
	"gopkg.in/yaml.v3"
)

const (
	ComposeActionUp      = "up"
	ComposeActionDown    = "down"
	ComposeActionIgnore  = "ignore"
	jsonSchemaTypeString = "string"
	jsonSchemaTypeObject = "object"
	pathDirMaxProperties = 7
)

var (
	errInvalidDirsMultiplePathKeys  = errors.New("invalid dirs item: multiple path keys found")
	errInvalidDirsExpectedMapOrNull = errors.New("invalid dirs item: expected mapping or null attributes")
	errInvalidDirsExpectedMapAttrs  = errors.New("invalid dirs item: expected mapping attributes")
	errInvalidDirsUnsupportedNode   = errors.New("invalid dirs item: unsupported YAML node kind")
	errInvalidDirsBoolValue         = errors.New("invalid dirs item key: expected boolean value")
	errDirPathRequired              = errors.New("path is required")
	errBecomeUserRequiresBecome     = errors.New("becomeUser requires become=true")
)

type DirConfig struct {
	Path       string `json:"path"                 yaml:"path"`
	Permission string `json:"permission,omitempty" yaml:"permission,omitempty"`
	Owner      string `json:"owner,omitempty"      yaml:"owner,omitempty"`
	Group      string `json:"group,omitempty"      yaml:"group,omitempty"`
	Become     bool   `json:"become,omitempty"     yaml:"become,omitempty"`
	BecomeUser string `json:"becomeUser,omitempty" yaml:"becomeUser,omitempty"`
	Recursive  bool   `json:"recursive,omitempty"  yaml:"recursive,omitempty"`
}

type dirConfigAttrsOnly struct {
	Permission string `yaml:"permission,omitempty"`
	Owner      string `yaml:"owner,omitempty"`
	Group      string `yaml:"group,omitempty"`
	Become     *bool  `yaml:"become,omitempty"`
	BecomeUser string `yaml:"becomeUser,omitempty"`
	Recursive  *bool  `yaml:"recursive,omitempty"`
}

type dirConfigSchemaProps = orderedmap.OrderedMap[string, *jsonschema.Schema]

func (d *DirConfig) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		d.Path = value.Value

		return nil
	case yaml.MappingNode:
		parsed, found, err := parseDirConfigPathKeyForm(value)
		if err != nil {
			return err
		}

		if found {
			*d = parsed

			return nil
		}
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return decodeDirConfigPlainNode(d, value)
	default:
		return decodeDirConfigPlainNode(d, value)
	}

	return decodeDirConfigPlainNode(d, value)
}

func decodeDirConfigPlainNode(dst *DirConfig, value *yaml.Node) error {
	type plain DirConfig

	return value.Decode((*plain)(dst))
}

func parseDirConfigPathKeyForm(value *yaml.Node) (DirConfig, bool, error) {
	var (
		cfg       DirConfig
		pathFound bool
		pathValue string
		attrs     dirConfigAttrsOnly
	)

	for i := 0; i+1 < len(value.Content); i += 2 {
		keyNode := value.Content[i]
		valNode := value.Content[i+1]
		key := keyNode.Value

		handled, err := mergeDirConfigKnownAttrs(key, valNode, &attrs)
		if err != nil {
			return cfg, false, err
		}

		if handled {
			continue
		}

		if pathFound {
			return cfg, false, fmt.Errorf("%w (%q and %q)", errInvalidDirsMultiplePathKeys, pathValue, key)
		}

		pathFound = true
		pathValue = key

		err = mergeDirConfigAttrsFromValue(pathValue, valNode, &attrs)
		if err != nil {
			return cfg, false, err
		}
	}

	if !pathFound {
		return cfg, false, nil
	}

	cfg.Path = pathValue
	cfg.Permission = attrs.Permission
	cfg.Owner = attrs.Owner

	cfg.Group = attrs.Group
	if attrs.Become != nil {
		cfg.Become = *attrs.Become
	}

	cfg.BecomeUser = attrs.BecomeUser
	if attrs.Recursive != nil {
		cfg.Recursive = *attrs.Recursive
	}

	return cfg, true, nil
}

func mergeDirConfigKnownAttrs(key string, valNode *yaml.Node, attrs *dirConfigAttrsOnly) (bool, error) {
	switch key {
	case "permission":
		attrs.Permission = valNode.Value

		return true, nil
	case "owner":
		attrs.Owner = valNode.Value

		return true, nil
	case "group":
		attrs.Group = valNode.Value

		return true, nil
	case "become":
		become, parseErr := parseDirConfigBoolAttr(key, valNode)
		if parseErr != nil {
			return true, parseErr
		}

		attrs.Become = &become

		return true, nil
	case "becomeUser":
		attrs.BecomeUser = valNode.Value

		return true, nil
	case "recursive":
		recursive, parseErr := parseDirConfigBoolAttr(key, valNode)
		if parseErr != nil {
			return true, parseErr
		}

		attrs.Recursive = &recursive

		return true, nil
	default:
		return false, nil
	}
}

func parseDirConfigBoolAttr(key string, valNode *yaml.Node) (bool, error) {
	var parsed bool

	err := valNode.Decode(&parsed)
	if err != nil {
		return false, fmt.Errorf("%w (%q)", errInvalidDirsBoolValue, key)
	}

	return parsed, nil
}

func mergeDirConfigAttrsFromValue(path string, valNode *yaml.Node, attrs *dirConfigAttrsOnly) error {
	switch valNode.Kind {
	case yaml.MappingNode:
		var nested dirConfigAttrsOnly

		err := valNode.Decode(&nested)
		if err != nil {
			return err
		}

		mergeNonEmptyDirConfigAttrs(attrs, nested)
	case yaml.ScalarNode:
		// Allow null/empty attributes:
		//   - <path>:
		if valNode.Tag != "!!null" && valNode.Value != "" {
			return fmt.Errorf("invalid dirs item for path %q: %w", path, errInvalidDirsExpectedMapOrNull)
		}
	case yaml.DocumentNode, yaml.SequenceNode, yaml.AliasNode:
		return fmt.Errorf("invalid dirs item for path %q: %w", path, errInvalidDirsExpectedMapAttrs)
	default:
		return fmt.Errorf("invalid dirs item for path %q: %w %d", path, errInvalidDirsUnsupportedNode, valNode.Kind)
	}

	return nil
}

func mergeNonEmptyDirConfigAttrs(dst *dirConfigAttrsOnly, src dirConfigAttrsOnly) {
	if src.Permission != "" {
		dst.Permission = src.Permission
	}

	if src.Owner != "" {
		dst.Owner = src.Owner
	}

	if src.Group != "" {
		dst.Group = src.Group
	}

	if src.Become != nil {
		become := *src.Become
		dst.Become = &become
	}

	if src.Recursive != nil {
		recursive := *src.Recursive
		dst.Recursive = &recursive
	}

	if src.BecomeUser != "" {
		dst.BecomeUser = src.BecomeUser
	}
}

func (*DirConfig) JSONSchema() *jsonschema.Schema {
	stringSchema := new(jsonschema.Schema)
	stringSchema.Type = jsonSchemaTypeString

	attrsProps := buildDirConfigAttrsProperties()
	pathKeyedObjectSchema := buildDirConfigPathKeyedObjectSchema(attrsProps)

	rootSchema := new(jsonschema.Schema)
	rootSchema.OneOf = []*jsonschema.Schema{stringSchema, pathKeyedObjectSchema}

	return rootSchema
}

func buildDirConfigAttrsProperties() *dirConfigSchemaProps {
	attrsProps := orderedmap.New[string, *jsonschema.Schema]()
	attrsProps.Set("permission", dirConfigStringOrIntegerSchema())
	attrsProps.Set("owner", dirConfigStringOrIntegerSchema())
	attrsProps.Set("group", dirConfigStringOrIntegerSchema())

	becomeSchema := new(jsonschema.Schema)
	becomeSchema.Type = "boolean"
	attrsProps.Set("become", becomeSchema)

	recursiveSchema := new(jsonschema.Schema)
	recursiveSchema.Type = "boolean"
	attrsProps.Set("recursive", recursiveSchema)

	becomeUserSchema := new(jsonschema.Schema)
	becomeUserSchema.Type = jsonSchemaTypeString
	attrsProps.Set("becomeUser", becomeUserSchema)

	return attrsProps
}

func dirConfigStringOrIntegerSchema() *jsonschema.Schema {
	schema := new(jsonschema.Schema)
	stringSchema := new(jsonschema.Schema)
	stringSchema.Type = jsonSchemaTypeString

	integerSchema := new(jsonschema.Schema)
	integerSchema.Type = "integer"

	schema.OneOf = []*jsonschema.Schema{
		stringSchema,
		integerSchema,
	}

	return schema
}

func buildDirConfigPathKeyedObjectSchema(attrsProps *dirConfigSchemaProps) *jsonschema.Schema {
	attrsObjectSchema := new(jsonschema.Schema)
	attrsObjectSchema.Type = jsonSchemaTypeObject
	attrsObjectSchema.Properties = attrsProps
	attrsObjectSchema.AdditionalProperties = jsonschema.FalseSchema

	nullSchema := new(jsonschema.Schema)
	nullSchema.Type = "null"

	pathValueSchema := new(jsonschema.Schema)
	pathValueSchema.OneOf = []*jsonschema.Schema{attrsObjectSchema, nullSchema}

	pathKeyPattern := "^(?!permission$|owner$|group$|become$|becomeUser$|recursive$).+$"
	pathPropertyMinCount := uint64(1)
	pathPropertyMaxCount := uint64(pathDirMaxProperties)

	pathKeyedObjectSchema := new(jsonschema.Schema)
	pathKeyedObjectSchema.Type = jsonSchemaTypeObject
	pathKeyedObjectSchema.Properties = attrsProps
	pathKeyedObjectSchema.PatternProperties = map[string]*jsonschema.Schema{pathKeyPattern: pathValueSchema}
	pathKeyedObjectSchema.AdditionalProperties = jsonschema.FalseSchema
	pathKeyedObjectSchema.MinProperties = &pathPropertyMinCount
	pathKeyedObjectSchema.MaxProperties = &pathPropertyMaxCount

	pathOnlyAttrsSchema := new(jsonschema.Schema)
	pathOnlyAttrsSchema.Type = jsonSchemaTypeObject
	pathOnlyAttrsSchema.Properties = attrsProps
	pathOnlyAttrsSchema.AdditionalProperties = jsonschema.FalseSchema
	pathKeyedObjectSchema.Not = pathOnlyAttrsSchema

	return pathKeyedObjectSchema
}

func ValidateDirConfigs(dirs []DirConfig) error {
	for i, dirConfig := range dirs {
		if dirConfig.Path == "" {
			return fmt.Errorf("dirs[%d]: %w", i, errDirPathRequired)
		}

		if dirConfig.Permission == "" {
		} else {
			_, err := strconv.ParseUint(dirConfig.Permission, 8, 32)
			if err != nil {
				return fmt.Errorf(
					"dirs[%d]: invalid permission %q (expected octal like \"0755\"): %w",
					i,
					dirConfig.Permission,
					err,
				)
			}
		}

		if dirConfig.BecomeUser != "" && !dirConfig.Become {
			return fmt.Errorf("dirs[%d]: %w", i, errBecomeUserRequiresBecome)
		}
	}

	return nil
}

func DefaultTemplateVarSources() []string {
	return []string{"*.yml", "*.yaml"}
}

type CmtConfig struct {
	BasePath         string            `json:"basePath"                   yaml:"basePath"`
	Defaults         *SyncDefaults     `json:"defaults,omitempty"         yaml:"defaults,omitempty"`
	Hosts            []HostEntry       `json:"hosts"                      yaml:"hosts"`
	BeforeApplyHooks *BeforeApplyHooks `json:"beforeApplyHooks,omitempty" yaml:"beforeApplyHooks,omitempty"`
}

type BeforeApplyHooks struct {
	BeforePlan        *HookCommand `json:"beforePlan,omitempty"        yaml:"beforePlan,omitempty"`
	BeforeApplyPrompt *HookCommand `json:"beforeApplyPrompt,omitempty" yaml:"beforeApplyPrompt,omitempty"`
	BeforeApply       *HookCommand `json:"beforeApply,omitempty"       yaml:"beforeApply,omitempty"`
}

type HookCommand struct {
	Command string `json:"command" yaml:"command"`
}

type SyncDefaults struct {
	RemotePath         string   `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string   `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string   `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	TemplateVarSources []string `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
}

type HostEntry struct {
	Name       string `json:"name"                 yaml:"name"`
	Host       string `json:"host"                 yaml:"host"`
	Port       int    `json:"port,omitempty"       yaml:"port,omitempty"`
	User       string `json:"user"                 yaml:"user"`
	SSHKeyPath string `json:"sshKeyPath,omitempty" yaml:"sshKeyPath,omitempty"`
	SSHAgent   bool   `json:"sshAgent,omitempty"   yaml:"sshAgent,omitempty"`

	ProxyCommand  string   `json:"-" yaml:"-"`
	IdentityFiles []string `json:"-" yaml:"-"`
	IdentityAgent string   `json:"-" yaml:"-"`
}

type HostConfig struct {
	SSHConfig          string                    `json:"sshConfig,omitempty"          yaml:"sshConfig,omitempty"`
	RemotePath         string                    `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string                    `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string                    `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	TemplateVarSources []string                  `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
	Projects           map[string]*ProjectConfig `json:"projects,omitempty"           yaml:"projects,omitempty"`
}

type ProjectConfig struct {
	RemotePath         string      `json:"remotePath,omitempty"         yaml:"remotePath,omitempty"`
	PostSyncCommand    string      `json:"postSyncCommand,omitempty"    yaml:"postSyncCommand,omitempty"`
	ComposeAction      string      `json:"composeAction,omitempty"      yaml:"composeAction,omitempty"`
	RemoveOrphans      bool        `json:"removeOrphans,omitempty"      yaml:"removeOrphans,omitempty"`
	Dirs               []DirConfig `json:"dirs,omitempty"               yaml:"dirs,omitempty"`
	TemplateVarSources []string    `json:"templateVarSources,omitempty" yaml:"templateVarSources,omitempty"`
}

type ResolvedProjectConfig struct {
	RemotePath         string
	PostSyncCommand    string
	ComposeAction      string
	RemoveOrphans      bool
	Dirs               []DirConfig
	TemplateVarSources []string
}

type HookConfigPaths struct {
	ConfigPath string `json:"configPath"`
	BasePath   string `json:"basePath"`
}

type BeforePlanHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

type BeforeApplyPromptHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

type BeforeApplyHookPayload struct {
	Hosts      []string        `json:"hosts"`
	WorkingDir string          `json:"workingDir"`
	Paths      HookConfigPaths `json:"paths"`
}

func ResolveProjectConfig(cmtDefaults *SyncDefaults, hostCfg *HostConfig, projectName string) ResolvedProjectConfig {
	resolved := resolveFromDefaults(cmtDefaults)
	if hostCfg == nil {
		resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)
		resolved.TemplateVarSources = normalizeTemplateVarSources(resolved.TemplateVarSources)

		return resolved
	}

	applyHostOverrides(&resolved, hostCfg)
	applyProjectOverrides(&resolved, hostCfg, projectName)
	resolved.ComposeAction = normalizeComposeAction(resolved.ComposeAction)
	resolved.TemplateVarSources = normalizeTemplateVarSources(resolved.TemplateVarSources)

	return resolved
}

func normalizeTemplateVarSources(sources []string) []string {
	if len(sources) == 0 {
		return DefaultTemplateVarSources()
	}

	return sources
}

func resolveFromDefaults(defaults *SyncDefaults) ResolvedProjectConfig {
	if defaults == nil {
		return ResolvedProjectConfig{
			RemotePath:         "",
			PostSyncCommand:    "",
			ComposeAction:      "",
			RemoveOrphans:      false,
			Dirs:               nil,
			TemplateVarSources: nil,
		}
	}

	return ResolvedProjectConfig{
		RemotePath:         defaults.RemotePath,
		PostSyncCommand:    defaults.PostSyncCommand,
		ComposeAction:      defaults.ComposeAction,
		RemoveOrphans:      false,
		Dirs:               nil,
		TemplateVarSources: defaults.TemplateVarSources,
	}
}

func applyHostOverrides(resolved *ResolvedProjectConfig, hostCfg *HostConfig) {
	if hostCfg.RemotePath != "" {
		resolved.RemotePath = hostCfg.RemotePath
	}

	if hostCfg.PostSyncCommand != "" {
		resolved.PostSyncCommand = hostCfg.PostSyncCommand
	}

	if hostCfg.ComposeAction != "" {
		resolved.ComposeAction = hostCfg.ComposeAction
	}

	if len(hostCfg.TemplateVarSources) > 0 {
		resolved.TemplateVarSources = hostCfg.TemplateVarSources
	}
}

func applyProjectOverrides(resolved *ResolvedProjectConfig, hostCfg *HostConfig, projectName string) {
	projectConfig, ok := hostCfg.Projects[projectName]
	if !ok || projectConfig == nil {
		return
	}

	if projectConfig.RemotePath != "" {
		resolved.RemotePath = projectConfig.RemotePath
	}

	if projectConfig.PostSyncCommand != "" {
		resolved.PostSyncCommand = projectConfig.PostSyncCommand
	}

	if projectConfig.ComposeAction != "" {
		resolved.ComposeAction = projectConfig.ComposeAction
	}

	resolved.RemoveOrphans = projectConfig.RemoveOrphans

	if len(projectConfig.Dirs) > 0 {
		resolved.Dirs = projectConfig.Dirs
	}

	if len(projectConfig.TemplateVarSources) > 0 {
		resolved.TemplateVarSources = projectConfig.TemplateVarSources
	}
}

func normalizeComposeAction(action string) string {
	if action == "" {
		return ComposeActionUp
	}

	return action
}
