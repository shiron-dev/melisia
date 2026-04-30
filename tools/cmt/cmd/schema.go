package cmd

import (
	"os"

	"github.com/shiron-dev/melisia/tools/cmt/internal/config"
	"github.com/spf13/cobra"
)

func newSchemaCmd() *cobra.Command {
	schemaCommand := new(cobra.Command)
	schemaCommand.Use = "schema [cmt|host|hook-before-plan|hook-before-apply-prompt|hook-before-apply]"
	schemaCommand.Short = "Generate JSON Schema for cmt config, host.yml, or hook stdin payloads"
	schemaCommand.Args = cobra.ExactArgs(1)
	schemaCommand.ValidArgs = config.SchemaKinds()
	schemaCommand.RunE = func(_ *cobra.Command, args []string) error {
		data, err := config.GenerateSchemaJSON(args[0])
		if err != nil {
			return err
		}

		_, err = os.Stdout.Write(append(data, '\n'))

		return err
	}

	return schemaCommand
}
