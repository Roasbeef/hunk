package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// rebaseControlOutput is the JSON output for rebase control commands.
type rebaseControlOutput struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	InProgress bool   `json:"in_progress"`
}

// NewRebaseContinueCmd creates the rebase continue command.
func NewRebaseContinueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Continue an in-progress rebase",
		Long: `Continue an interactive rebase after resolving conflicts.

Before running this command:
  1. Resolve any conflicts in the affected files
  2. Stage the resolved files with 'git add <file>'

If there are still unresolved conflicts, this command will fail.`,
		Example: `  # After resolving conflicts
  git add resolved-file.go
  hunk rebase continue`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebaseControl(
				cmd.Context(), cmd.OutOrStdout(), "continue",
			)
		},
	}
}

// NewRebaseAbortCmd creates the rebase abort command.
func NewRebaseAbortCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "abort",
		Short: "Abort an in-progress rebase",
		Long: `Abort the current interactive rebase and restore the original branch.

This will discard all progress made during the rebase and return
the branch to its original state before the rebase started.`,
		Example: `  # Abort the current rebase
  hunk rebase abort`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebaseControl(
				cmd.Context(), cmd.OutOrStdout(), "abort",
			)
		},
	}
}

// NewRebaseSkipCmd creates the rebase skip command.
func NewRebaseSkipCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skip",
		Short: "Skip the current commit during rebase",
		Long: `Skip the current commit and continue with the next one.

Use this when a commit cannot be applied cleanly and you want
to drop it from the rebased history rather than resolving conflicts.`,
		Example: `  # Skip the problematic commit
  hunk rebase skip`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRebaseControl(
				cmd.Context(), cmd.OutOrStdout(), "skip",
			)
		},
	}
}

func runRebaseControl(ctx context.Context, w io.Writer, action string) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	// First check if rebase is in progress.
	status, err := executor.RebaseStatus(ctx)
	if err != nil {
		return err
	}

	if !status.InProgress {
		return fmt.Errorf("no rebase in progress")
	}

	// Perform the action.
	switch action {
	case "continue":
		err = executor.RebaseContinue(ctx)
	case "abort":
		err = executor.RebaseAbort(ctx)
	case "skip":
		err = executor.RebaseSkip(ctx)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	if err != nil {
		return err
	}

	// Check new status.
	newStatus, err := executor.RebaseStatus(ctx)
	if err != nil {
		return err
	}

	if cfg.JSONOut {
		return formatRebaseControlJSON(w, action, newStatus)
	}

	return formatRebaseControlText(w, action, newStatus)
}

func formatRebaseControlJSON(
	w io.Writer, action string, state *git.RebaseState,
) error {

	output := rebaseControlOutput{
		Success:    true,
		InProgress: state.InProgress,
	}

	switch action {
	case "continue":
		if state.InProgress {
			output.Message = fmt.Sprintf(
				"Continued. %d commits remaining.",
				state.RemainingCount,
			)
		} else {
			output.Message = "Rebase completed successfully."
		}
	case "abort":
		output.Message = "Rebase aborted. Branch restored to original state."
	case "skip":
		if state.InProgress {
			output.Message = fmt.Sprintf(
				"Skipped commit. %d commits remaining.",
				state.RemainingCount,
			)
		} else {
			output.Message = "Skipped commit. Rebase completed."
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

func formatRebaseControlText(
	w io.Writer, action string, state *git.RebaseState,
) error {

	switch action {
	case "continue":
		if state.InProgress {
			fmt.Fprintf(w, "Continued. %d commits remaining.\n",
				state.RemainingCount)
		} else {
			fmt.Fprintln(w, "Rebase completed successfully.")
		}
	case "abort":
		fmt.Fprintln(w, "Rebase aborted. Branch restored to original state.")
	case "skip":
		if state.InProgress {
			fmt.Fprintf(w, "Skipped commit. %d commits remaining.\n",
				state.RemainingCount)
		} else {
			fmt.Fprintln(w, "Skipped commit. Rebase completed.")
		}
	}

	return nil
}
