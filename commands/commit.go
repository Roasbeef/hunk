package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// NewCommitCmd creates the commit command.
func NewCommitCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Commit staged changes",
		Long: `Create a commit with the currently staged changes.

This is a thin wrapper around 'git commit' for convenience.`,
		Example: `  # Commit with a message
  hunk commit -m "add error handling"

  # Stage and commit in one command
  hunk stage main.go:10-20 && hunk commit -m "fix bug"`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if message == "" {
				return fmt.Errorf("commit message required (-m)")
			}

			return runCommit(cmd.Context(), cmd.OutOrStdout(), message)
		},
	}

	cmd.Flags().StringVarP(
		&message, "message", "m", "",
		"commit message",
	)
	_ = cmd.MarkFlagRequired("message")

	return cmd
}

func runCommit(ctx context.Context, w io.Writer, message string) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	// Check if there are staged changes.
	diffText, err := executor.DiffCached(ctx)
	if err != nil {
		return err
	}

	if diffText == "" {
		return fmt.Errorf("nothing staged for commit")
	}

	if err := executor.Commit(ctx, message); err != nil {
		return err
	}

	fmt.Fprintln(w, "Committed successfully.")

	return nil
}
