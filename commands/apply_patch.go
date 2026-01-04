package commands

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// NewApplyPatchCmd creates the apply-patch command.
func NewApplyPatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply-patch [file]",
		Short: "Apply a patch to the staging area",
		Long: `Apply a unified diff patch to the staging area.

If no file is specified, reads from stdin.
This is equivalent to 'git apply --cached'.`,
		Example: `  # Apply patch from file
  hunk apply-patch changes.patch

  # Apply patch from stdin (useful for piping)
  cat changes.patch | hunk apply-patch`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApplyPatch(cmd.Context(), cmd.OutOrStdout(), args)
		},
	}

	return cmd
}

func runApplyPatch(ctx context.Context, w io.Writer, args []string) error {
	cfg := getConfig(ctx)

	var input io.Reader

	if len(args) == 0 {
		input = os.Stdin
	} else {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open patch file: %w", err)
		}
		defer f.Close()

		input = f
	}

	executor := git.NewShellExecutor(cfg.WorkDir)

	if err := executor.ApplyPatch(ctx, input); err != nil {
		return err
	}

	fmt.Fprintln(w, "Patch applied to staging area.")

	return nil
}
