// Package commands contains the CLI command implementations.
package commands

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

// configKey is the context key for runtime config.
type configKey struct{}

// Config holds runtime configuration for commands.
type Config struct {
	WorkDir string
	JSONOut bool
}

// getConfig retrieves config from context, or returns defaults.
func getConfig(ctx context.Context) Config {
	if cfg, ok := ctx.Value(configKey{}).(Config); ok {
		return cfg
	}

	return Config{}
}

// NewRootCmd creates the root command.
func NewRootCmd() *cobra.Command {
	var (
		workDir string
		jsonOut bool
	)

	cmd := &cobra.Command{
		Use:     "hunk",
		Short:   "Sparse partial commits for AI agents",
		Version: Version,
		Long: `Hunk enables precise, line-level staging for git commits.

Designed for AI agents that need to make surgical changes to codebases,
hunk provides a simple interface for selecting and staging specific lines
from a diff.

Examples:
  # Show all changes with line numbers
  hunk diff

  # Show changes in JSON format (for agents)
  hunk diff --json

  # Stage specific lines from a file
  hunk stage main.go:10-20

  # Stage multiple ranges from multiple files
  hunk stage main.go:10-20,30-40 utils.go:5-15

  # Preview what's staged
  hunk preview

  # Commit staged changes
  hunk commit -m "add error handling"

  # Apply a patch directly to staging
  hunk apply-patch < changes.diff`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Store config in context for subcommands.
			cfg := Config{
				WorkDir: workDir,
				JSONOut: jsonOut,
			}
			ctx := context.WithValue(cmd.Context(), configKey{}, cfg)
			cmd.SetContext(ctx)
		},
	}

	cmd.PersistentFlags().StringVarP(
		&workDir, "dir", "C", "",
		"run as if git was started in this directory",
	)
	cmd.PersistentFlags().BoolVar(
		&jsonOut, "json", false,
		"output in JSON format (for machine consumption)",
	)

	// Add subcommands.
	cmd.AddCommand(NewDiffCmd())
	cmd.AddCommand(NewStageCmd())
	cmd.AddCommand(NewPreviewCmd())
	cmd.AddCommand(NewCommitCmd())
	cmd.AddCommand(NewResetCmd())
	cmd.AddCommand(NewApplyPatchCmd())
	cmd.AddCommand(NewVersionCmd())

	return cmd
}

// Execute runs the root command.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
