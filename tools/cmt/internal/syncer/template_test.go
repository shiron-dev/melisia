package syncer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
)

// ---------------------------------------------------------------------------
// parseSecretsYAML
// ---------------------------------------------------------------------------

func TestParseSecretsYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	secretsPath := filepath.Join(dir, "env.secrets.yml")

	err := os.WriteFile(secretsPath, []byte(`github_client_id: abc123
github_client_secret: secret456
smtp_port: 587
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := parseSecretsYAML(secretsPath)
	if err != nil {
		t.Fatal(err)
	}

	if vars["github_client_id"] != "abc123" {
		t.Errorf("github_client_id = %v", vars["github_client_id"])
	}

	if vars["github_client_secret"] != "secret456" {
		t.Errorf("github_client_secret = %v", vars["github_client_secret"])
	}
	// YAML の整数は int としてパースされます。
	if vars["smtp_port"] != 587 {
		t.Errorf("smtp_port = %v (%T)", vars["smtp_port"], vars["smtp_port"])
	}
}

func TestParseSecretsYAML_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantLen int
		wantErr bool
	}{
		{
			name: "not exist",
			setup: func(t *testing.T) string {
				t.Helper()

				return "/nonexistent/env.secrets.yml"
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "invalid YAML",
			setup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				secretsPath := filepath.Join(dir, "env.secrets.yml")

				err := os.WriteFile(secretsPath, []byte(`{invalid: yaml: [}`), 0600)
				if err != nil {
					t.Fatal(err)
				}

				return secretsPath
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.setup(t)

			vars, err := parseSecretsYAML(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseSecretsYAML() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && len(vars) != tt.wantLen {
				t.Errorf("expected %d vars, got %v", tt.wantLen, vars)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadTemplateVars
// ---------------------------------------------------------------------------

func TestLoadTemplateVars_LaterFileOverridesEarlier(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "a_base.yml"), []byte("KEY1: from_base\nSHARED: base_value\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	overrideContent := []byte("KEY2: from_override\nSHARED: override_value\n")

	err = os.WriteFile(filepath.Join(hostProjectDir, "b_override.yml"), overrideContent, 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", config.DefaultTemplateVarSources())
	if err != nil {
		t.Fatal(err)
	}

	if vars["KEY1"] != "from_base" {
		t.Errorf("KEY1 = %v", vars["KEY1"])
	}

	if vars["KEY2"] != "from_override" {
		t.Errorf("KEY2 = %v", vars["KEY2"])
	}

	if vars["SHARED"] != "override_value" {
		t.Errorf("SHARED = %v, want override_value", vars["SHARED"])
	}
}

func TestLoadTemplateVars_NoFiles(t *testing.T) {
	t.Parallel()

	base := t.TempDir()

	err := os.MkdirAll(filepath.Join(base, "hosts", "server1", "grafana"), 0750)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", config.DefaultTemplateVarSources())
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %d: %v", len(vars), vars)
	}
}

func TestLoadTemplateVars_SingleYAMLFile(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml"), []byte("secret_key: s3cret\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", config.DefaultTemplateVarSources())
	if err != nil {
		t.Fatal(err)
	}

	if vars["secret_key"] != "s3cret" {
		t.Errorf("secret_key = %v, want s3cret", vars["secret_key"])
	}
}

func TestLoadTemplateVars_CustomSourcePattern(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "vars.toml.yml"), []byte("key: custom\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "ignored.yml"), []byte("other: skip\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", []string{"vars.toml.yml"})
	if err != nil {
		t.Fatal(err)
	}

	if vars["key"] != "custom" {
		t.Errorf("key = %v, want custom", vars["key"])
	}

	if _, ok := vars["other"]; ok {
		t.Error("ignored.yml should not be loaded with custom pattern")
	}
}

func TestLoadTemplateVars_ExcludesComposeOverride(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "compose.override.yml"), []byte("excluded: yes\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", config.DefaultTemplateVarSources())
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected 0 vars, got %d: %v", len(vars), vars)
	}
}

func TestLoadTemplateVars_ExcludesSOPSFiles(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	hostProjectDir := filepath.Join(base, "hosts", "server1", "grafana")

	err := os.MkdirAll(hostProjectDir, 0750)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(hostProjectDir, "env.secrets.yml.sops"), []byte("secret_key: ENC[encrypted]\n"), 0600)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(
		filepath.Join(hostProjectDir, "cloudflare-access.sops.yml"),
		[]byte("client_secret: ENC[encrypted]\n"),
		0600,
	)
	if err != nil {
		t.Fatal(err)
	}

	vars, err := LoadTemplateVars(base, "server1", "grafana", config.DefaultTemplateVarSources())
	if err != nil {
		t.Fatal(err)
	}

	if len(vars) != 0 {
		t.Errorf("expected encrypted SOPS files to be excluded, got %d vars: %v", len(vars), vars)
	}
}

// ---------------------------------------------------------------------------
// RenderTemplate
// ---------------------------------------------------------------------------

func TestRenderTemplate(t *testing.T) {
	t.Parallel()

	data := []byte(`host = {{ .smtp_host }}
password = {{ .smtp_password }}`)

	vars := map[string]any{
		"smtp_host":     "mail.example.com:587",
		"smtp_password": "s3cret",
	}

	result, err := RenderTemplate(data, vars)
	if err != nil {
		t.Fatal(err)
	}

	expected := `host = mail.example.com:587
password = s3cret`
	if string(result) != expected {
		t.Errorf("got %q, want %q", string(result), expected)
	}
}

func TestRenderTemplate_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       []byte
		vars       map[string]any
		wantErr    bool
		wantResult []byte
	}{
		{
			name:       "no vars",
			data:       []byte("plain text with no templates"),
			vars:       nil,
			wantErr:    false,
			wantResult: []byte("plain text with no templates"),
		},
		{
			name:       "empty vars",
			data:       []byte("plain text"),
			vars:       map[string]any{},
			wantErr:    false,
			wantResult: []byte("plain text"),
		},
		{
			name:       "binary skipped",
			data:       []byte("binary\x00content"),
			vars:       map[string]any{"key": "val"},
			wantErr:    false,
			wantResult: []byte("binary\x00content"),
		},
		{
			name:    "missing key error",
			data:    []byte("value = {{ .missing_key }}"),
			vars:    map[string]any{"other_key": "val"},
			wantErr: true,
		},
		{
			name:    "missing key error with no vars",
			data:    []byte("TUNNEL_TOKEN={{ .cf_tunnel_token }}"),
			vars:    nil,
			wantErr: true,
		},
		{
			name:    "invalid template",
			data:    []byte("bad {{ .unterminated"),
			vars:    map[string]any{"key": "val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := RenderTemplate(tt.data, tt.vars)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenderTemplate() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && string(result) != string(tt.wantResult) {
				t.Errorf("RenderTemplate() = %q, want %q", string(result), string(tt.wantResult))
			}
		})
	}
}

func TestRenderTemplate_ComplexTemplate(t *testing.T) {
	t.Parallel()

	data := []byte(`services:
  app:
    environment:
      - DB_HOST={{ .db_host }}
      - DB_PORT={{ .db_port }}
      - DB_NAME={{ .db_name }}`)

	vars := map[string]any{
		"db_host": "postgres.local",
		"db_port": 5432,
		"db_name": "myapp",
	}

	result, err := RenderTemplate(data, vars)
	if err != nil {
		t.Fatal(err)
	}

	expected := `services:
  app:
    environment:
      - DB_HOST=postgres.local
      - DB_PORT=5432
      - DB_NAME=myapp`
	if string(result) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", string(result), expected)
	}
}
