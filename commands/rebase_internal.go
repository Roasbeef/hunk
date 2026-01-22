package commands

import (
	"fmt"
	"os"

	"github.com/roasbeef/hunk/rebase"
	"github.com/spf13/cobra"
)

// newApplyRebaseSpecCmd creates the hidden internal command for applying specs.
// This is invoked by git as GIT_SEQUENCE_EDITOR to transform the todo file.
func newApplyRebaseSpecCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_apply-spec SPECFILE TODOFILE",
		Hidden: true,
		Short:  "Internal command to apply rebase spec (invoked by git)",
		Args:   cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			specFile := args[0]
			todoFile := args[1]

			return applyRebaseSpec(specFile, todoFile)
		},
	}
}

// applyRebaseSpec reads the spec file and transforms the todo file.
func applyRebaseSpec(specFile, todoFile string) error {
	// Read the spec.
	specData, err := os.ReadFile(specFile)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	spec, err := rebase.ParseSpec(specData)
	if err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	// Read the original todo file.
	todoData, err := os.ReadFile(todoFile)
	if err != nil {
		return fmt.Errorf("failed to read todo file: %w", err)
	}

	// Parse the original todo to get full commit info.
	originalEntries := rebase.ParseTodoFile(string(todoData))
	if len(originalEntries) == 0 {
		return fmt.Errorf("no commits found in rebase todo")
	}

	// Validate spec against available commits.
	if err := spec.ValidateAgainstCommits(originalEntries); err != nil {
		return err
	}

	// Reorder and transform entries to match spec.
	newEntries, err := rebase.ReorderToMatchSpec(spec, originalEntries)
	if err != nil {
		return err
	}

	// Generate the new todo file content.
	newTodo := rebase.GenerateTodoFromEntries(newEntries)

	// Write back to the todo file.
	if err := os.WriteFile(todoFile, []byte(newTodo), 0600); err != nil {
		return fmt.Errorf("failed to write todo file: %w", err)
	}

	return nil
}
