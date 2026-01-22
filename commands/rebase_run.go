package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/roasbeef/hunk/git"
	"github.com/roasbeef/hunk/rebase"
	"github.com/spf13/cobra"
)

// rebaseRunOutput is the JSON output for rebase run.
type rebaseRunOutput struct {
	Success     bool   `json:"success"`
	Message     string `json:"message"`
	InProgress  bool   `json:"in_progress,omitempty"`
	HasConflict bool   `json:"has_conflict,omitempty"`
}

// NewRebaseRunCmd creates the rebase run command.
func NewRebaseRunCmd() *cobra.Command {
	var (
		onto     string
		specFile string
	)

	cmd := &cobra.Command{
		Use:   "run [ACTIONS]",
		Short: "Execute an interactive rebase with specified actions",
		Long: `Execute an interactive rebase using a declarative specification.

Actions can be specified either as command-line arguments or via a JSON file.

CLI Syntax:
  Comma-separated list of actions. Each action can be:
  - A commit hash (defaults to 'pick')
  - ACTION:COMMIT (e.g., squash:abc123)
  - ACTION:COMMIT:MESSAGE (e.g., reword:abc123:Better message)

Available actions:
  pick    - Use commit as-is
  reword  - Use commit but change message
  edit    - Use commit but stop for amending
  squash  - Combine with previous commit (concat messages)
  fixup   - Combine with previous commit (discard message)
  drop    - Remove commit from history
  exec    - Run shell command (e.g., exec:make test)

JSON Syntax (with --spec):
  {
    "actions": [
      {"action": "pick", "commit": "abc123"},
      {"action": "squash", "commit": "def456", "message": "Combined"}
    ]
  }`,
		Example: `  # Rebase all commits as picks
  hunk rebase run --onto main abc123,def456,ghi789

  # Squash second and third commits into first
  hunk rebase run --onto main pick:abc123,squash:def456,squash:ghi789

  # Drop a commit
  hunk rebase run --onto main pick:abc123,drop:def456,pick:ghi789

  # Reword a commit
  hunk rebase run --onto main "reword:abc123:Better commit message"

  # From JSON file
  hunk rebase run --onto main --spec rebase-plan.json

  # From stdin
  echo '{"actions":[...]}' | hunk rebase run --onto main --spec -`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if onto == "" {
				return fmt.Errorf("--onto is required")
			}

			return runRebaseRun(
				cmd.Context(), cmd.OutOrStdout(),
				onto, specFile, args,
			)
		},
	}

	cmd.Flags().StringVar(
		&onto, "onto", "",
		"base reference to rebase onto (required)",
	)
	cmd.Flags().StringVar(
		&specFile, "spec", "",
		"JSON file containing rebase specification (use - for stdin)",
	)

	_ = cmd.MarkFlagRequired("onto")

	return cmd
}

func runRebaseRun(
	ctx context.Context, w io.Writer,
	onto, specFile string, args []string,
) error {
	cfg := getConfig(ctx)

	// Parse the spec from file or CLI args.
	spec, err := parseRebaseSpec(specFile, args)
	if err != nil {
		return err
	}

	executor := git.NewShellExecutor(cfg.WorkDir)

	// First verify commits exist.
	commits, err := executor.RebaseList(ctx, onto)
	if err != nil {
		return fmt.Errorf("failed to list commits: %w", err)
	}

	if len(commits) == 0 {
		return fmt.Errorf(
			"no commits to rebase: HEAD is already at or behind %s", onto,
		)
	}

	// Create temp file for the spec.
	specData, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to serialize spec: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "hunk-rebase-spec-*.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(specData); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)

		return fmt.Errorf("failed to write spec: %w", err)
	}

	tmpFile.Close()

	// Cleanup after RebaseStart completes. Git invokes GIT_SEQUENCE_EDITOR
	// synchronously during the rebase, so _apply-spec will have read the
	// spec file before RebaseStart returns.
	defer os.Remove(tmpPath)

	// Get the hunk binary path for the sequence editor.
	hunkPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Construct the editor command.
	editor := fmt.Sprintf("%s rebase _apply-spec %s", hunkPath, tmpPath)

	// Start the rebase.
	if err := executor.RebaseStart(ctx, onto, editor); err != nil {
		return err
	}

	// Check final status.
	state, err := executor.RebaseStatus(ctx)
	if err != nil {
		return err
	}

	if cfg.JSONOut {
		return formatRebaseRunJSON(w, state)
	}

	return formatRebaseRunText(w, state)
}

func parseRebaseSpec(specFile string, args []string) (*rebase.Spec, error) {
	// If spec file provided, use that.
	if specFile != "" {
		var data []byte
		var err error

		if specFile == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(specFile)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to read spec file: %w", err)
		}

		return rebase.ParseSpec(data)
	}

	// Otherwise parse CLI args.
	if len(args) == 0 {
		return nil, fmt.Errorf("no actions specified; provide commits/actions or --spec")
	}

	return rebase.ParseCLISpec(args)
}

func formatRebaseRunJSON(w io.Writer, state *git.RebaseState) error {
	output := rebaseRunOutput{
		Success:     !state.InProgress,
		InProgress:  state.InProgress,
		HasConflict: state.State == git.RebaseStateConflict,
	}

	if state.InProgress {
		if state.State == git.RebaseStateConflict {
			output.Message = "Rebase paused due to conflicts"
		} else {
			output.Message = "Rebase in progress"
		}
	} else {
		output.Message = "Rebase completed successfully"
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

func formatRebaseRunText(w io.Writer, state *git.RebaseState) error {
	if !state.InProgress {
		fmt.Fprintln(w, "Rebase completed successfully.")

		return nil
	}

	if state.State == git.RebaseStateConflict {
		fmt.Fprintln(w, "Rebase paused due to conflicts.")
		fmt.Fprintln(w, "")

		for _, c := range state.Conflicts {
			fmt.Fprintf(w, "  Conflict: %s\n", c.Path)
		}

		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Resolve conflicts, then:")
		fmt.Fprintln(w, "  hunk rebase continue  # to continue")
		fmt.Fprintln(w, "  hunk rebase abort     # to abort")
	} else {
		fmt.Fprintf(w, "Rebase in progress. %d commits remaining.\n",
			state.RemainingCount)
	}

	return nil
}
