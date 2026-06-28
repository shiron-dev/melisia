package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	rootCommand.AddCommand(newForceUnlockCmd(&configPath))

	return rootCommand
}

// Execute runs the root command.
//
// It installs a signal-aware context so that Ctrl+C (SIGINT) or SIGTERM cancels
// any in-flight external command (ssh/scp/docker). Without this every command
// ran under context.Background() and a hung or unreachable host would block the
// whole run with no way to interrupt it short of killing the process.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCommand := newRootCmd()
	rootCommand.SetArgs(normalizeTerraformTargetArgs(os.Args[1:]))

	return rootCommand.ExecuteContext(ctx)
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
