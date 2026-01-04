package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/git"
	"github.com/roasbeef/hunk/patch"
	"github.com/spf13/cobra"
)

// NewStageCmd creates the stage command.
func NewStageCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "stage FILE:LINES [FILE:LINES...]",
		Short: "Stage specific lines",
		Long: `Stage specific lines from the working directory.

Lines are specified using FILE:LINES syntax where LINES can be:
  - A single line number: main.go:42
  - A range: main.go:10-20
  - Multiple ranges: main.go:10-20,30,40-50

Line numbers refer to the NEW file (after changes).
Use 'hunk diff' to see line numbers.`,
		Example: `  # Stage lines 10-20 from main.go
  hunk stage main.go:10-20

  # Stage multiple ranges from one file
  hunk stage main.go:10-20,30-40

  # Stage from multiple files
  hunk stage main.go:10-20 utils.go:5-15

  # Preview what would be staged
  hunk stage --dry-run main.go:10-20`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStage(cmd.Context(), cmd.OutOrStdout(), args, dryRun)
		},
	}

	cmd.Flags().BoolVar(
		&dryRun, "dry-run", false,
		"show what would be staged without staging",
	)

	return cmd
}

func runStage(ctx context.Context, w io.Writer, args []string, dryRun bool) error {
	// Parse all selections.
	selections, err := diff.ParseSelections(args)
	if err != nil {
		return fmt.Errorf("invalid selection: %w", err)
	}

	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	// Get the current diff.
	diffText, err := executor.Diff(ctx)
	if err != nil {
		return err
	}

	if diffText == "" {
		return fmt.Errorf("no unstaged changes")
	}

	parsed, err := diff.Parse(diffText)
	if err != nil {
		return err
	}

	// Generate a patch for the selected lines.
	patchBytes, err := patch.Generate(parsed, selections)
	if err != nil {
		return err
	}

	if len(patchBytes) == 0 {
		return fmt.Errorf("no matching lines found for selection")
	}

	if dryRun {
		fmt.Fprint(w, string(patchBytes))

		return nil
	}

	// Apply the patch to the staging area.
	if err := executor.ApplyPatch(ctx, bytes.NewReader(patchBytes)); err != nil {
		return fmt.Errorf("failed to stage changes: %w", err)
	}

	fmt.Fprintln(w, "Changes staged successfully.")

	return nil
}
