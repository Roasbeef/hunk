package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/roasbeef/hunk/git"
	"github.com/spf13/cobra"
)

// rebaseListOutput is the JSON output for rebase list.
type rebaseListOutput struct {
	Base    string             `json:"base"`
	Head    string             `json:"head"`
	Commits []commitInfoOutput `json:"commits"`
	Count   int                `json:"count"`
}

// commitInfoOutput is the JSON output for commit info.
type commitInfoOutput struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
	Subject   string `json:"subject"`
	Author    string `json:"author"`
	Date      string `json:"date"`
	Position  int    `json:"position"`
}

// NewRebaseListCmd creates the rebase list command.
func NewRebaseListCmd() *cobra.Command {
	var onto string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List commits that would be rebased",
		Long: `List commits that would be rebased onto the specified base.

This shows all commits from the base reference to HEAD that would be
included in an interactive rebase.

Use --json for machine-readable output.`,
		Example: `  # List commits from main to HEAD
  hunk rebase list --onto main

  # JSON output for agents
  hunk rebase list --onto main --json

  # List relative to a specific commit
  hunk rebase list --onto HEAD~5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if onto == "" {
				return fmt.Errorf("--onto is required")
			}

			return runRebaseList(cmd.Context(), cmd.OutOrStdout(), onto)
		},
	}

	cmd.Flags().StringVar(&onto, "onto", "", "base reference to rebase onto (required)")
	cmd.MarkFlagRequired("onto")

	return cmd
}

func runRebaseList(ctx context.Context, w io.Writer, onto string) error {
	cfg := getConfig(ctx)
	executor := git.NewShellExecutor(cfg.WorkDir)

	commits, err := executor.RebaseList(ctx, onto)
	if err != nil {
		return err
	}

	if cfg.JSONOut {
		return formatRebaseListJSON(w, onto, commits)
	}

	return formatRebaseListText(w, onto, commits)
}

func formatRebaseListJSON(w io.Writer, onto string, commits []git.CommitInfo) error {
	output := rebaseListOutput{
		Base:    onto,
		Head:    "HEAD",
		Commits: make([]commitInfoOutput, len(commits)),
		Count:   len(commits),
	}

	for i, c := range commits {
		output.Commits[i] = commitInfoOutput{
			Hash:      c.Hash,
			ShortHash: c.ShortHash,
			Subject:   c.Subject,
			Author:    c.Author,
			Date:      c.Date.Format("2006-01-02T15:04:05Z07:00"),
			Position:  i + 1,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

func formatRebaseListText(w io.Writer, onto string, commits []git.CommitInfo) error {
	if len(commits) == 0 {
		fmt.Fprintf(w, "No commits to rebase onto %s\n", onto)

		return nil
	}

	fmt.Fprintf(w, "%d commit(s) to rebase onto %s:\n\n", len(commits), onto)

	for i, c := range commits {
		label := ""
		if i == len(commits)-1 {
			label = " (HEAD)"
		}

		fmt.Fprintf(w, "%d. %s %s%s\n", i+1, c.ShortHash, c.Subject, label)
	}

	return nil
}
