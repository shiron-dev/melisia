package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateSchemaJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		schemaType     string
		wantErr        bool
		wantErrMsg     string
		wantFields     []string
		wantJSONFields []string
	}{
		{
			name:           "cmt schema",
			schemaType:     "cmt",
			wantErr:        false,
			wantJSONFields: []string{"$schema", "$defs"},
			wantFields:     []string{"basePath", "hosts", "beforeApplyHooks"},
		},
		{
			name:       "host schema",
			schemaType: "host",
			wantErr:    false,
			wantFields: []string{"remotePath", "projects", "removeOrphans", "templateVarSources"},
		},
		{
			name:       "hook-before-plan schema",
			schemaType: "hook-before-plan",
			wantErr:    false,
			wantFields: []string{"hosts", "workingDir", "paths"},
		},
		{
			name:       "hook-before-apply-prompt schema",
			schemaType: "hook-before-apply-prompt",
			wantErr:    false,
			wantFields: []string{"hosts", "workingDir", "paths"},
		},
		{
			name:       "hook-before-apply schema",
			schemaType: "hook-before-apply",
			wantErr:    false,
			wantFields: []string{"hosts", "workingDir", "paths"},
		},
		{
			name:       "unknown schema type",
			schemaType: "unknown",
			wantErr:    true,
			wantErrMsg: "unknown schema type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := GenerateSchemaJSON(tt.schemaType)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateSchemaJSON(%q) error = %v, wantErr %v", tt.schemaType, err, tt.wantErr)
			}

			if tt.wantErr {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error message = %q, want to contain %q", err.Error(), tt.wantErrMsg)
				}

				return
			}

			var obj map[string]any

			err = json.Unmarshal(data, &obj)
			if err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}

			for _, field := range tt.wantJSONFields {
				if _, ok := obj[field]; !ok {
					t.Errorf("missing JSON field %q", field)
				}
			}

			raw := string(data)
			for _, field := range tt.wantFields {
				if !strings.Contains(raw, field) {
					t.Errorf("schema should reference %q", field)
				}
			}
		})
	}
}

func TestDirConfigJSONSchema(t *testing.T) {
	t.Parallel()

	schema := new(DirConfig).JSONSchema()

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}

	raw := string(data)
	if strings.Contains(raw, `"required":["path"]`) {
		t.Fatalf("legacy dirs path object format should not be allowed in schema: %s", raw)
	}

	if !strings.Contains(raw, `"minProperties":1`) || !strings.Contains(raw, `"maxProperties":7`) {
		t.Fatalf("dirs path-keyed object property-count constraints are missing: %s", raw)
	}

	if strings.Contains(raw, `"maxProperties":1`) {
		t.Fatalf("dirs schema should allow owner/group/permission alongside path key: %s", raw)
	}

	if !strings.Contains(raw, `"patternProperties"`) ||
		!strings.Contains(raw, `"^(?!permission$|owner$|group$|become$|becomeUser$|recursive$).+$"`) {
		t.Fatalf("dirs schema should constrain path keys via patternProperties: %s", raw)
	}

	if !strings.Contains(raw, `"type":"null"`) {
		t.Fatalf("dirs schema should allow null value for '- <path>:' form: %s", raw)
	}

	if !strings.Contains(raw, `"type":"integer"`) {
		t.Fatalf("dirs schema should allow integer values for owner/group/permission: %s", raw)
	}

	if !strings.Contains(raw, `"become"`) || !strings.Contains(raw, `"becomeUser"`) {
		t.Fatalf("dirs schema should include become/becomeUser fields: %s", raw)
	}

	if !strings.Contains(raw, `"recursive"`) {
		t.Fatalf("dirs schema should include recursive field: %s", raw)
	}
}

func TestHostSchema_AllowsNullProjectConfig(t *testing.T) {
	t.Parallel()

	data, err := GenerateSchemaJSON("host")
	if err != nil {
		t.Fatalf("GenerateSchemaJSON(host): %v", err)
	}

	var root map[string]any

	err = json.Unmarshal(data, &root)
	if err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	defs, ok := root["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("missing $defs in host schema")
	}

	hostDef, ok := defs["HostConfig"].(map[string]any)
	if !ok {
		t.Fatalf("missing HostConfig definition in host schema")
	}

	properties, ok := hostDef["properties"].(map[string]any)
	if !ok {
		t.Fatalf("missing HostConfig.properties in host schema")
	}

	projects, ok := properties["projects"].(map[string]any)
	if !ok {
		t.Fatalf("missing HostConfig.properties.projects in host schema")
	}

	additionalProperties, ok := projects["additionalProperties"].(map[string]any)
	if !ok {
		t.Fatalf("host schema projects.additionalProperties should be an object")
	}

	oneOf, ok := additionalProperties["oneOf"].([]any)
	if !ok {
		t.Fatalf("host schema projects.additionalProperties should include oneOf")
	}

	hasNull := false
	hasProjectRef := false

	for _, branch := range oneOf {
		branchSchema, ok := branch.(map[string]any)
		if !ok {
			continue
		}

		if branchType, ok := branchSchema["type"].(string); ok && branchType == "null" {
			hasNull = true
		}

		if branchRef, ok := branchSchema["$ref"].(string); ok && branchRef == "#/$defs/ProjectConfig" {
			hasProjectRef = true
		}
	}

	if !hasNull || !hasProjectRef {
		t.Fatalf(
			"host schema projects.additionalProperties should allow ProjectConfig or null, got: %v",
			additionalProperties,
		)
	}
}
