package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

var ErrUnknownSchemaType = errors.New("unknown schema type")
var (
	ErrHostSchemaMissingDefs                 = errors.New("host schema patch failed: missing $defs")
	ErrHostSchemaMissingHostConfig           = errors.New("host schema patch failed: missing HostConfig")
	ErrHostSchemaMissingHostConfigProperties = errors.New("host schema patch failed: missing HostConfig.properties")
	ErrHostSchemaMissingProjects             = errors.New(
		"host schema patch failed: missing HostConfig.properties.projects",
	)
	ErrHostSchemaMissingAdditionalProperties = errors.New(
		"host schema patch failed: missing projects.additionalProperties",
	)
)

func SchemaKinds() []string {
	return []string{"cmt", "host", "hook-before-plan", "hook-before-apply-prompt", "hook-before-apply"}
}

func GenerateSchemaJSON(kind string) ([]byte, error) {
	var target any

	switch kind {
	case "cmt":
		targetConfig := new(CmtConfig)
		targetConfig.BasePath = ""
		targetConfig.Defaults = nil
		targetConfig.Hosts = nil
		targetConfig.BeforeApplyHooks = nil
		target = targetConfig
	case "host":
		targetHostConfig := new(HostConfig)
		targetHostConfig.SSHConfig = ""
		targetHostConfig.RemotePath = ""
		targetHostConfig.PostSyncCommand = ""
		targetHostConfig.Projects = nil
		target = targetHostConfig
	case "hook-before-plan":
		target = new(BeforePlanHookPayload)
	case "hook-before-apply-prompt":
		target = new(BeforeApplyPromptHookPayload)
	case "hook-before-apply":
		target = new(BeforeApplyHookPayload)
	default:
		return nil, fmt.Errorf("%w %q (valid: %v)", ErrUnknownSchemaType, kind, SchemaKinds())
	}

	reflector := new(jsonschema.Reflector)
	reflector.Mapper = func(t reflect.Type) *jsonschema.Schema {
		if t == reflect.TypeFor[DirConfig]() {
			return new(DirConfig).JSONSchema()
		}

		return nil
	}

	schema := reflector.Reflect(target)

	data, err := marshalSchemaWithCompatibility(kind, schema)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func marshalSchemaWithCompatibility(kind string, schema any) ([]byte, error) {
	rawData, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshalling schema: %w", err)
	}

	if kind != "host" {
		indentedData, marshalErr := json.MarshalIndent(schema, "", "  ")
		if marshalErr != nil {
			return nil, fmt.Errorf("marshalling schema: %w", marshalErr)
		}

		return indentedData, nil
	}

	var schemaDoc map[string]any

	err = json.Unmarshal(rawData, &schemaDoc)
	if err != nil {
		return nil, fmt.Errorf("decoding schema document: %w", err)
	}

	err = allowNullHostProjectOverrides(schemaDoc)
	if err != nil {
		return nil, err
	}

	indentedData, marshalErr := json.MarshalIndent(schemaDoc, "", "  ")
	if marshalErr != nil {
		return nil, fmt.Errorf("marshalling schema: %w", marshalErr)
	}

	return indentedData, nil
}

func allowNullHostProjectOverrides(schemaDoc map[string]any) error {
	defs, ok := schemaDoc["$defs"].(map[string]any)
	if !ok {
		return ErrHostSchemaMissingDefs
	}

	hostConfigDef, ok := defs["HostConfig"].(map[string]any)
	if !ok {
		return ErrHostSchemaMissingHostConfig
	}

	properties, ok := hostConfigDef["properties"].(map[string]any)
	if !ok {
		return ErrHostSchemaMissingHostConfigProperties
	}

	projects, ok := properties["projects"].(map[string]any)
	if !ok {
		return ErrHostSchemaMissingProjects
	}

	projectConfigSchema, ok := projects["additionalProperties"]
	if !ok {
		return ErrHostSchemaMissingAdditionalProperties
	}

	projects["additionalProperties"] = map[string]any{
		"oneOf": []any{
			projectConfigSchema,
			map[string]any{"type": "null"},
		},
	}

	return nil
}
