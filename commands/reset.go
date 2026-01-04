package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// NewResetCmd creates the reset command.
func NewResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset [files...]",
		Short: "Unstage changes",
		Long: `Unstage all staged changes, or specific files if specified.

This is equivalent to 'git reset HEAD'.`,
		Example: `  # Unstage all changes
  hunk reset

  # Unstage specific file
  hunk reset main.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReset(cmd.Context(), cmd.OutOrStdout(), args)
		},
	}

	return cmd
}

func runReset(ctx context.Context, w io.Writer, paths []string) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	if len(paths) == 0 {
		if err := executor.Reset(ctx); err != nil {
			return err
		}

		fmt.Fprintln(w, "Unstaged all changes.")
	} else {
		for _, path := range paths {
			if err := executor.ResetPath(ctx, path); err != nil {
				return err
			}
		}

		fmt.Fprintf(w, "Unstaged %d file(s).\n", len(paths))
	}

	return nil
}
