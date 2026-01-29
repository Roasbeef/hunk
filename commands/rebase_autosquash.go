package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/roasbeef/hunk/git"
	"github.com/roasbeef/hunk/rebase"
	"github.com/spf13/cobra"
)

// autosquashOutput is the JSON output for autosquash.
type autosquashOutput struct {
	Success       bool               `json:"success"`
	Message       string             `json:"message"`
	FixupsApplied int                `json:"fixups_applied"`
	Actions       []autosquashAction `json:"actions,omitempty"`
	InProgress    bool               `json:"in_progress,omitempty"`
	HasConflict   bool               `json:"has_conflict,omitempty"`
}

type autosquashAction struct {
	Action string `json:"action"`
	Commit string `json:"commit"`
	Target string `json:"target,omitempty"`
}

// NewRebaseAutosquashCmd creates the rebase autosquash command.
func NewRebaseAutosquashCmd() *cobra.Command {
	var (
		onto    string
		dryRun  bool
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "autosquash",
		Short: "Rebase with automatic fixup squashing",
		Long: `Rebase with automatic reordering and squashing of fixup commits.

This command identifies commits with subjects starting with "fixup! " or
"squash! ", matches them to their target commits, and creates a rebase
plan that places each fixup/squash commit immediately after its target.

This is equivalent to 'git rebase -i --autosquash' but non-interactive.`,
		Example: `  # Autosquash all fixup commits onto main
  hunk rebase autosquash --onto main

  # Dry run to see what would happen
  hunk rebase autosquash --onto main --dry-run

  # Verbose output showing the plan
  hunk rebase autosquash --onto main --verbose`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if onto == "" {
				return fmt.Errorf("--onto is required")
			}

			return runRebaseAutosquash(
				cmd.Context(), cmd.OutOrStdout(),
				onto, dryRun, verbose,
			)
		},
	}

	cmd.Flags().StringVar(
		&onto, "onto", "",
		"base reference to rebase onto (required)",
	)
	cmd.Flags().BoolVar(
		&dryRun, "dry-run", false,
		"show what would be done without executing",
	)
	cmd.Flags().BoolVarP(
		&verbose, "verbose", "v", false,
		"show detailed plan before executing",
	)

	_ = cmd.MarkFlagRequired("onto")

	return cmd
}

func runRebaseAutosquash(
	ctx context.Context, w io.Writer,
	onto string, dryRun, verbose bool,
) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	// Get the list of commits to rebase.
	commits, err := executor.RebaseList(ctx, onto)
	if err != nil {
		return fmt.Errorf("failed to list commits: %w", err)
	}

	if len(commits) == 0 {
		if cfg.JSONOut {
			return json.NewEncoder(w).Encode(autosquashOutput{
				Success: true,
				Message: "No commits to rebase",
			})
		}

		fmt.Fprintln(w, "No commits to rebase.")

		return nil
	}

	// Build the autosquash plan.
	plan, fixupCount := buildAutosquashPlan(commits)

	if fixupCount == 0 {
		if cfg.JSONOut {
			return json.NewEncoder(w).Encode(autosquashOutput{
				Success:       true,
				Message:       "No fixup/squash commits found",
				FixupsApplied: 0,
			})
		}

		fmt.Fprintln(w, "No fixup/squash commits found. Nothing to do.")

		return nil
	}

	// Dry run - just show the plan.
	if dryRun {
		return formatAutosquashDryRun(w, cfg.JSONOut, plan, fixupCount)
	}

	// Verbose - show plan before executing.
	if verbose && !cfg.JSONOut {
		fmt.Fprintf(w, "Autosquash plan (%d fixups):\n\n", fixupCount)

		for _, action := range plan.Actions {
			fmt.Fprintf(w, "  %s %s\n", action.Action, action.Commit)
		}

		fmt.Fprintln(w, "")
	}

	// Execute the rebase.
	return executeAutosquashRebase(ctx, w, &cfg, executor, onto, plan, fixupCount)
}

// buildAutosquashPlan creates a rebase spec with fixups reordered after targets.
func buildAutosquashPlan(commits []git.CommitInfo) (*rebase.Spec, int) {
	// First pass: identify fixups and their targets.
	entries := make([]commitEntry, 0, len(commits))
	subjectToHash := make(map[string]string)

	// Build subject index from non-fixup commits.
	for _, c := range commits {
		if !isFixupCommit(c.Subject) && !isSquashCommit(c.Subject) {
			subjectToHash[c.Subject] = c.Hash
		}
	}

	// Second pass: create entries with target matching.
	fixupCount := 0

	for _, c := range commits {
		entry := commitEntry{info: c, action: rebase.ActionPick}

		if isFixupCommit(c.Subject) {
			entry.action = rebase.ActionFixup
			entry.target = findTargetCommit(
				c.Subject, "fixup! ", subjectToHash, commits,
			)
			fixupCount++
		} else if isSquashCommit(c.Subject) {
			entry.action = rebase.ActionSquash
			entry.target = findTargetCommit(
				c.Subject, "squash! ", subjectToHash, commits,
			)
			fixupCount++
		}

		entries = append(entries, entry)
	}

	// Reorder entries: place fixups right after their targets.
	reordered := reorderForAutosquash(entries)

	// Build the spec.
	actions := make([]rebase.Action, 0, len(reordered))
	for _, e := range reordered {
		actions = append(actions, rebase.Action{
			Action: e.action,
			Commit: e.info.Hash,
		})
	}

	return &rebase.Spec{Actions: actions}, fixupCount
}

// isFixupCommit checks if a subject starts with "fixup! ".
func isFixupCommit(subject string) bool {
	return strings.HasPrefix(subject, "fixup! ")
}

// isSquashCommit checks if a subject starts with "squash! ".
func isSquashCommit(subject string) bool {
	return strings.HasPrefix(subject, "squash! ")
}

// findTargetCommit finds the commit hash that a fixup/squash targets.
func findTargetCommit(
	subject, prefix string,
	subjectToHash map[string]string,
	commits []git.CommitInfo,
) string {
	targetSubject := strings.TrimPrefix(subject, prefix)

	// Try exact match first.
	if hash, ok := subjectToHash[targetSubject]; ok {
		return hash
	}

	// Try prefix match.
	for subj, hash := range subjectToHash {
		if strings.HasPrefix(subj, targetSubject) {
			return hash
		}
	}

	// Check for nested fixups (fixup! fixup! ...).
	if strings.HasPrefix(targetSubject, "fixup! ") ||
		strings.HasPrefix(targetSubject, "squash! ") {

		return findTargetCommit(targetSubject, "fixup! ", subjectToHash, commits)
	}

	return ""
}

// reorderForAutosquash places fixups right after their target commits.
func reorderForAutosquash(entries []commitEntry) []commitEntry {
	result := make([]commitEntry, 0, len(entries))

	// Track which entries have been placed.
	placed := make(map[string]bool)

	// Collect fixups by their target.
	fixupsByTarget := make(map[string][]commitEntry)

	for _, e := range entries {
		if e.target != "" {
			fixupsByTarget[e.target] = append(fixupsByTarget[e.target], e)
		}
	}

	// Place commits in order, inserting fixups after their targets.
	for _, e := range entries {
		if placed[e.info.Hash] {
			continue
		}

		// Skip if this is a fixup (will be placed after its target).
		if e.target != "" {
			continue
		}

		// Place this commit.
		result = append(result, e)
		placed[e.info.Hash] = true

		// Place any fixups targeting this commit.
		for _, fixup := range fixupsByTarget[e.info.Hash] {
			if !placed[fixup.info.Hash] {
				result = append(result, fixup)
				placed[fixup.info.Hash] = true
			}
		}
	}

	// Place any remaining fixups (targets not found - place at end).
	for _, e := range entries {
		if !placed[e.info.Hash] {
			result = append(result, e)
		}
	}

	return result
}

type commitEntry struct {
	info   git.CommitInfo
	action rebase.ActionType
	target string
}

func executeAutosquashRebase(
	ctx context.Context, w io.Writer,
	cfg *Config, executor *git.ShellExecutor,
	onto string, plan *rebase.Spec, fixupCount int,
) error {
	// Create temp file for the spec.
	specData, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("failed to serialize spec: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "hunk-autosquash-spec-*.json")
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
	defer os.Remove(tmpPath)

	// Get the hunk binary path for the sequence editor.
	hunkPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	editor := formatAutosquashEditorCommand(hunkPath, tmpPath)

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
		return formatAutosquashJSON(w, state, fixupCount)
	}

	return formatAutosquashText(w, state, fixupCount)
}

func formatAutosquashEditorCommand(hunkPath, specPath string) string {
	if runtime.GOOS == "windows" {
		hunkPath = strings.ReplaceAll(hunkPath, "\\", "/")
		specPath = strings.ReplaceAll(specPath, "\\", "/")

		return fmt.Sprintf(`"%s" rebase _apply-spec "%s"`, hunkPath, specPath)
	}

	return fmt.Sprintf("%s rebase _apply-spec %s", hunkPath, specPath)
}

func formatAutosquashDryRun(
	w io.Writer, jsonOut bool,
	plan *rebase.Spec, fixupCount int,
) error {
	if jsonOut {
		actions := make([]autosquashAction, 0, len(plan.Actions))

		for _, a := range plan.Actions {
			actions = append(actions, autosquashAction{
				Action: string(a.Action),
				Commit: a.Commit,
			})
		}

		return json.NewEncoder(w).Encode(autosquashOutput{
			Success:       true,
			Message:       "Dry run - no changes made",
			FixupsApplied: fixupCount,
			Actions:       actions,
		})
	}

	fmt.Fprintf(w, "Dry run: would apply %d fixup(s)\n\n", fixupCount)

	for _, a := range plan.Actions {
		fmt.Fprintf(w, "  %s %s\n", a.Action, a.Commit[:7])
	}

	return nil
}

func formatAutosquashJSON(
	w io.Writer, state *git.RebaseState, fixupCount int,
) error {
	output := autosquashOutput{
		Success:       !state.InProgress,
		FixupsApplied: fixupCount,
		InProgress:    state.InProgress,
		HasConflict:   state.State == git.RebaseStateConflict,
	}

	if state.InProgress {
		if state.State == git.RebaseStateConflict {
			output.Message = "Autosquash paused due to conflicts"
		} else {
			output.Message = "Autosquash in progress"
		}
	} else {
		output.Message = fmt.Sprintf(
			"Autosquash completed: %d fixup(s) squashed", fixupCount,
		)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

func formatAutosquashText(
	w io.Writer, state *git.RebaseState, fixupCount int,
) error {
	if !state.InProgress {
		fmt.Fprintf(w, "Autosquash completed: %d fixup(s) squashed.\n",
			fixupCount)

		return nil
	}

	if state.State == git.RebaseStateConflict {
		fmt.Fprintln(w, "Autosquash paused due to conflicts.")
		fmt.Fprintln(w, "")

		for _, c := range state.Conflicts {
			fmt.Fprintf(w, "  Conflict: %s\n", c.Path)
		}

		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Resolve conflicts, then:")
		fmt.Fprintln(w, "  hunk rebase continue  # to continue")
		fmt.Fprintln(w, "  hunk rebase abort     # to abort")
	} else {
		fmt.Fprintf(w, "Autosquash in progress. %d commits remaining.\n",
			state.RemainingCount)
	}

	return nil
}
