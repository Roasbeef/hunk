package commands

import (
	"github.com/spf13/cobra"
)

// NewRebaseCmd creates the rebase parent command.
func NewRebaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rebase",
		Short: "Perform interactive rebases programmatically",
		Long: `Perform interactive rebases without interactive prompts.

This command group provides an agent-friendly API for git interactive rebases.
Instead of using an interactive editor, you specify the rebase operations
declaratively.

The workflow is:
  1. Use 'hunk rebase list' to see commits that would be rebased
  2. Use 'hunk rebase run' with a specification of actions
  3. If conflicts occur, resolve them and use 'hunk rebase continue'
  4. Or use 'hunk rebase abort' to cancel

Examples:
  # List commits that would be rebased onto main
  hunk rebase list --onto main

  # Rebase with default actions (pick all)
  hunk rebase run --onto main abc123,def456

  # Squash commits together
  hunk rebase run --onto main pick:abc123,squash:def456

  # Drop a commit
  hunk rebase run --onto main pick:abc123,drop:def456`,
	}

	// Add subcommands.
	cmd.AddCommand(NewRebaseListCmd())
	cmd.AddCommand(NewRebaseStatusCmd())
	cmd.AddCommand(NewRebaseRunCmd())
	cmd.AddCommand(NewRebaseContinueCmd())
	cmd.AddCommand(NewRebaseAbortCmd())
	cmd.AddCommand(NewRebaseSkipCmd())
	cmd.AddCommand(newApplyRebaseSpecCmd())

	return cmd
}
