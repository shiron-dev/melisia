package syncer

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"text/template"

	"gopkg.in/yaml.v3"
)

func isTemplateVarExcluded(name string) bool {
	return name == "compose.override.yml" || name == "host.yml"
}

func LoadTemplateVars(basePath, hostName, projectName string, sources []string) (map[string]any, error) {
	hostProjectDir := filepath.Join(basePath, "hosts", hostName, projectName)
	vars := make(map[string]any)

	var matched []string

	for _, pattern := range sources {
		paths, err := filepath.Glob(filepath.Join(hostProjectDir, pattern))
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}

		matched = append(matched, paths...)
	}

	sort.Strings(matched)

	seen := make(map[string]bool, len(matched))

	for _, filePath := range matched {
		if seen[filePath] {
			continue
		}

		seen[filePath] = true

		if isTemplateVarExcluded(filepath.Base(filePath)) {
			continue
		}

		info, statErr := os.Stat(filePath)
		if statErr != nil || info.IsDir() {
			continue
		}

		fileVars, err := parseSecretsYAML(filePath)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, err)
		}

		maps.Copy(vars, fileVars)
	}

	return vars, nil
}

func parseSecretsYAML(path string) (map[string]any, error) {
	vars := make(map[string]any)
	cleanPath := filepath.Clean(path)

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return vars, nil
		}

		return nil, err
	}

	raw := make(map[string]any)

	unmarshalErr := yaml.Unmarshal(data, &raw)
	if unmarshalErr != nil {
		return nil, fmt.Errorf("invalid YAML: %w", unmarshalErr)
	}

	maps.Copy(vars, raw)

	return vars, nil
}

func RenderTemplate(data []byte, vars map[string]any) ([]byte, error) {
	if isBinary(data) {
		return data, nil
	}

	if len(vars) == 0 {
		return data, nil
	}

	tmpl, err := template.New("").Option("missingkey=error").Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer

	executeErr := tmpl.Execute(&buf, vars)
	if executeErr != nil {
		return nil, fmt.Errorf("template render error: %w", executeErr)
	}

	return buf.Bytes(), nil
}
