package cmd

import (
	"bytes"
	"encoding/csv"
	"strings"

	"github.com/spf13/cobra"
)

type sharedStringSliceValue struct {
	value   *[]string
	changed bool
}

func bindProjectFilterFlags(command *cobra.Command, projectFilter *[]string) {
	value := newSharedStringSliceValue(nil, projectFilter)
	command.Flags().Var(value, "project", "filter by project name (repeatable)")
	command.Flags().Var(value, "target", "alias of --project: filter by project name (repeatable)")
}

func newSharedStringSliceValue(defaultValue []string, target *[]string) *sharedStringSliceValue {
	*target = defaultValue

	return &sharedStringSliceValue{
		value:   target,
		changed: false,
	}
}

func (v *sharedStringSliceValue) Set(raw string) error {
	values, err := readCSVFlagValue(raw)
	if err != nil {
		return err
	}

	if !v.changed {
		*v.value = values
	} else {
		*v.value = append(*v.value, values...)
	}

	v.changed = true

	return nil
}

func (v *sharedStringSliceValue) String() string {
	raw, err := writeCSVFlagValue(*v.value)
	if err != nil {
		return ""
	}

	return "[" + raw + "]"
}

func (v *sharedStringSliceValue) Type() string {
	return "stringSlice"
}

func readCSVFlagValue(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}

	reader := csv.NewReader(strings.NewReader(raw))

	return reader.Read()
}

func writeCSVFlagValue(values []string) (string, error) {
	buffer := new(bytes.Buffer)
	writer := csv.NewWriter(buffer)

	err := writer.Write(values)
	if err != nil {
		return "", err
	}

	writer.Flush()

	return strings.TrimSuffix(buffer.String(), "\n"), nil
}
