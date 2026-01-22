package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// rebaseStatusOutput is the JSON output for rebase status.
type rebaseStatusOutput struct {
	InProgress       bool                 `json:"in_progress"`
	State            string               `json:"state"`
	CurrentAction    string               `json:"current_action,omitempty"`
	TotalCommits     int                  `json:"total_commits,omitempty"`
	RemainingCommits int                  `json:"remaining_commits,omitempty"`
	CompletedCommits int                  `json:"completed_commits,omitempty"`
	Conflicts        []conflictInfoOutput `json:"conflicts,omitempty"`
	OriginalBranch   string               `json:"original_branch,omitempty"`
	OntoRef          string               `json:"onto_ref,omitempty"`
	Instructions     []string             `json:"instructions,omitempty"`
}

// conflictInfoOutput is the JSON output for conflict info.
type conflictInfoOutput struct {
	File         string `json:"file"`
	ConflictType string `json:"conflict_type"`
}

// NewRebaseStatusCmd creates the rebase status command.
func NewRebaseStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show current rebase status",
		Long: `Show the current status of an interactive rebase.

If a rebase is in progress, this shows:
- Whether there are conflicts
- How many commits remain
- What files have conflicts

Use --json for machine-readable output.`,
		Example: `  # Check rebase status
  hunk rebase status

  # JSON output for agents
  hunk rebase status --json`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRebaseStatus(cmd.Context(), cmd.OutOrStdout())
		},
	}

	return cmd
}

func runRebaseStatus(ctx context.Context, w io.Writer) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	state, err := executor.RebaseStatus(ctx)
	if err != nil {
		return err
	}

	if cfg.JSONOut {
		return formatRebaseStatusJSON(w, state)
	}

	return formatRebaseStatusText(w, state)
}

func formatRebaseStatusJSON(w io.Writer, state *git.RebaseState) error {
	output := rebaseStatusOutput{
		InProgress:       state.InProgress,
		State:            string(state.State),
		TotalCommits:     state.TotalCount,
		RemainingCommits: state.RemainingCount,
		CompletedCommits: state.CompletedCount,
		OriginalBranch:   state.OriginalBranch,
		OntoRef:          state.OntoRef,
	}

	if state.CurrentAction != "" {
		output.CurrentAction = state.CurrentAction
	}

	if len(state.Conflicts) > 0 {
		output.Conflicts = make([]conflictInfoOutput, len(state.Conflicts))

		for i, c := range state.Conflicts {
			output.Conflicts[i] = conflictInfoOutput{
				File:         c.Path,
				ConflictType: c.ConflictType,
			}
		}

		output.Instructions = []string{
			"Resolve conflicts in the listed files",
			"Stage resolved files with 'git add <file>'",
			"Continue with 'hunk rebase continue'",
			"Or abort with 'hunk rebase abort'",
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

func formatRebaseStatusText(w io.Writer, state *git.RebaseState) error {
	if !state.InProgress {
		fmt.Fprintln(w, "No rebase in progress.")

		return nil
	}

	fmt.Fprintf(w, "Rebase in progress on %s\n", state.OriginalBranch)
	fmt.Fprintf(w, "  Rebasing onto: %s\n", state.OntoRef)
	fmt.Fprintf(w, "  Progress: %d/%d commits\n",
		state.CompletedCount, state.TotalCount)

	if state.State == git.RebaseStateConflict {
		fmt.Fprintln(w, "\nConflicts:")

		for _, c := range state.Conflicts {
			fmt.Fprintf(w, "  - %s (%s)\n", c.Path, c.ConflictType)
		}

		fmt.Fprintln(w, "\nResolve conflicts, stage with 'git add', then:")
		fmt.Fprintln(w, "  hunk rebase continue  # to continue")
		fmt.Fprintln(w, "  hunk rebase skip      # to skip this commit")
		fmt.Fprintln(w, "  hunk rebase abort     # to abort")
	}

	return nil
}
