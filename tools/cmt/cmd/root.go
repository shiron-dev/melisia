package cmd

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	rootCommand := new(cobra.Command)
	rootCommand.Use = "cmt"
	rootCommand.Short = "Compose Manage Tool — push-based sync for Docker Compose projects"
	rootCommand.SilenceUsage = true
	rootCommand.SilenceErrors = true
	rootCommand.Long = `cmt is a source-of-truth, push-based tool that syncs Docker Compose
project files from a local repository to remote hosts via SSH.

It follows a plan/apply workflow similar to Terraform:
  cmt plan   — show what would change
  cmt apply  — apply changes (with confirmation)`

	var configPath string

	var debugEnabled bool

	rootCommand.PersistentFlags().StringVar(&configPath, "config", "config.yml", "path to cmt config file")
	rootCommand.PersistentFlags().BoolVar(&debugEnabled, "debug", false, "enable debug logging")

	rootCommand.PersistentPreRun = func(_ *cobra.Command, _ []string) {
		handlerOptions := new(slog.HandlerOptions)
		if debugEnabled {
			handlerOptions.Level = slog.LevelDebug
		} else {
			handlerOptions.Level = slog.LevelWarn
		}

		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, handlerOptions)))
	}

	rootCommand.AddCommand(newPlanCmd(&configPath))
	rootCommand.AddCommand(newApplyCmd(&configPath))
	rootCommand.AddCommand(newSchemaCmd())

	return rootCommand
}

// Execute runs the root command.
func Execute() error {
	rootCommand := newRootCmd()
	rootCommand.SetArgs(normalizeTerraformTargetArgs(os.Args[1:]))

	return rootCommand.Execute()
}

func normalizeTerraformTargetArgs(args []string) []string {
	normalized := make([]string, len(args))
	for i, arg := range args {
		switch {
		case arg == "-target":
			normalized[i] = "--target"
		case strings.HasPrefix(arg, "-target="):
			normalized[i] = "--target=" + strings.TrimPrefix(arg, "-target=")
		default:
			normalized[i] = arg
		}
	}

	return normalized
}
