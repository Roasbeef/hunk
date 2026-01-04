package commands

import (
	"context"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/git"
	"github.com/roasbeef/hunk/output"
	"github.com/spf13/cobra"
)

// NewDiffCmd creates the diff command.
func NewDiffCmd() *cobra.Command {
	var (
		staged      bool
		showRaw     bool
		showFiles   bool
		showSummary bool
		showStage   bool
	)

	cmd := &cobra.Command{
		Use:   "diff [files...]",
		Short: "Show changes with line numbers",
		Long: `Show unstaged (or staged) changes with line numbers.

Each line is prefixed with its line number in the new file,
making it easy to specify line ranges for staging.

Use --json for machine-readable output suitable for AI agents.`,
		Example: `  # Show all unstaged changes
  hunk diff

  # Show changes for specific files
  hunk diff main.go utils.go

  # Show staged changes
  hunk diff --staged

  # JSON output for AI agents
  hunk diff --json

  # Show suggested stage commands
  hunk diff --stage-hints`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiff(cmd.Context(), cmd.OutOrStdout(), args, diffOptions{
				staged:      staged,
				showRaw:     showRaw,
				showFiles:   showFiles,
				showSummary: showSummary,
				showStage:   showStage,
			})
		},
	}

	cmd.Flags().BoolVar(
		&staged, "staged", false,
		"show staged changes instead of unstaged",
	)
	cmd.Flags().BoolVar(
		&showRaw, "raw", false,
		"show raw unified diff",
	)
	cmd.Flags().BoolVar(
		&showFiles, "files", false,
		"show only file names",
	)
	cmd.Flags().BoolVar(
		&showSummary, "summary", false,
		"show summary statistics",
	)
	cmd.Flags().BoolVar(
		&showStage, "stage-hints", false,
		"show suggested hunk stage commands",
	)

	return cmd
}

type diffOptions struct {
	staged      bool
	showRaw     bool
	showFiles   bool
	showSummary bool
	showStage   bool
}

func runDiff(ctx context.Context, w io.Writer, paths []string, opts diffOptions) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	var diffText string
	var err error

	if opts.staged {
		diffText, err = executor.DiffCached(ctx, paths...)
	} else {
		diffText, err = executor.Diff(ctx, paths...)
	}

	if err != nil {
		return err
	}

	// Get untracked files for awareness (only for unstaged diffs).
	var untracked []string
	if !opts.staged {
		status, statusErr := executor.Status(ctx)
		if statusErr == nil && len(status.UntrackedFiles) > 0 {
			untracked = status.UntrackedFiles
		}
	}

	if diffText == "" {
		if cfg.JSONOut {
			return output.FormatJSONEmptyWithUntracked(w, untracked)
		}

		if len(untracked) > 0 {
			fmt.Fprintf(w, "(%d untracked file(s) not shown - use git add)\n",
				len(untracked))
		}

		return nil
	}

	parsed, err := diff.Parse(diffText)
	if err != nil {
		return err
	}

	if cfg.JSONOut {
		return output.FormatJSONWithUntracked(w, parsed, untracked)
	}

	// Handle different output modes.
	var formatErr error
	switch {
	case opts.showRaw:
		formatErr = output.FormatRaw(w, parsed)
	case opts.showFiles:
		formatErr = output.FormatFileList(w, parsed)
	case opts.showSummary:
		formatErr = output.FormatTextSummary(w, parsed)
	case opts.showStage:
		formatErr = output.FormatStagingCommands(w, parsed)
	default:
		formatErr = output.FormatText(w, parsed, output.DefaultTextOptions())
	}

	if formatErr != nil {
		return formatErr
	}

	// Show note about untracked files.
	if len(untracked) > 0 && !opts.showRaw {
		fmt.Fprintf(w, "\n(%d untracked file(s) not shown - use git add)\n",
			len(untracked))
	}

	return nil
}
