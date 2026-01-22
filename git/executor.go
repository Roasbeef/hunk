package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ShellExecutor implements Executor by shelling out to git.
type ShellExecutor struct {
	// WorkDir is the working directory for git commands.
	// If empty, uses current directory.
	WorkDir string
}

// NewShellExecutor creates a new ShellExecutor.
func NewShellExecutor(workDir string) *ShellExecutor {
	return &ShellExecutor{WorkDir: workDir}
}

// run executes a git command and returns stdout.
func (e *ShellExecutor) run(
	ctx context.Context, stdin io.Reader, args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = stdin

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf(
			"git %s failed: %w: %s",
			strings.Join(args, " "), err, stderr.String(),
		)
	}

	return stdout.String(), nil
}

// Diff returns the unified diff for unstaged changes.
func (e *ShellExecutor) Diff(
	ctx context.Context, paths ...string,
) (string, error) {
	args := []string{"diff", "--no-color"}
	args = append(args, paths...)

	return e.run(ctx, nil, args...)
}

// DiffCached returns the unified diff for staged changes.
func (e *ShellExecutor) DiffCached(
	ctx context.Context, paths ...string,
) (string, error) {
	args := []string{"diff", "--cached", "--no-color"}
	args = append(args, paths...)

	return e.run(ctx, nil, args...)
}

// ApplyPatch applies a patch to the staging area.
func (e *ShellExecutor) ApplyPatch(
	ctx context.Context, patch io.Reader,
) error {
	_, err := e.run(ctx, patch, "apply", "--cached", "-")

	return err
}

// Commit creates a commit with the given message.
func (e *ShellExecutor) Commit(ctx context.Context, message string) error {
	_, err := e.run(ctx, nil, "commit", "-m", message)

	return err
}

// Reset unstages all staged changes.
func (e *ShellExecutor) Reset(ctx context.Context) error {
	_, err := e.run(ctx, nil, "reset", "HEAD")

	return err
}

// ResetPath unstages changes for a specific path.
func (e *ShellExecutor) ResetPath(ctx context.Context, path string) error {
	_, err := e.run(ctx, nil, "reset", "HEAD", "--", path)

	return err
}

// Status returns the current repository status.
func (e *ShellExecutor) Status(ctx context.Context) (*RepoStatus, error) {
	output, err := e.run(ctx, nil, "status", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}

	status := &RepoStatus{}

	// Parse porcelain output. Format: XY PATH\0
	// X = staged status, Y = unstaged status.
	entries := strings.Split(output, "\x00")
	for _, entry := range entries {
		if len(entry) < 3 {
			continue
		}

		staged := entry[0]
		unstaged := entry[1]
		path := entry[3:]

		switch {
		case staged == '?' && unstaged == '?':
			status.UntrackedFiles = append(status.UntrackedFiles, path)
		case staged != ' ' && staged != '?':
			status.StagedFiles = append(status.StagedFiles, path)
		case unstaged != ' ':
			status.UnstagedFiles = append(status.UnstagedFiles, path)
		}
	}

	return status, nil
}

// Root returns the repository root directory.
func (e *ShellExecutor) Root(ctx context.Context) (string, error) {
	output, err := e.run(ctx, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(output), nil
}

// gitDir returns the git directory path. This correctly handles worktrees
// where .git is a file pointing to the actual git directory.
func (e *ShellExecutor) gitDir(ctx context.Context) (string, error) {
	output, err := e.run(ctx, nil, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}

	gitDir := strings.TrimSpace(output)

	// If the path is relative, make it absolute relative to WorkDir.
	if !filepath.IsAbs(gitDir) && e.WorkDir != "" {
		gitDir = filepath.Join(e.WorkDir, gitDir)
	}

	return gitDir, nil
}

// RebaseList returns commits that would be rebased onto the given base.
func (e *ShellExecutor) RebaseList(
	ctx context.Context, base string,
) ([]CommitInfo, error) {
	// Use git log to list commits from base..HEAD.
	// Format: hash|short_hash|subject|author|date
	format := "%H|%h|%s|%an <%ae>|%aI"
	output, err := e.run(
		ctx, nil,
		"log", "--format="+format, "--reverse", base+"..HEAD",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var commits []CommitInfo

	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		date, _ := time.Parse(time.RFC3339, parts[4])

		commits = append(commits, CommitInfo{
			Hash:      parts[0],
			ShortHash: parts[1],
			Subject:   parts[2],
			Author:    parts[3],
			Date:      date,
		})
	}

	return commits, nil
}

// RebaseStart begins an interactive rebase with a custom sequence editor.
func (e *ShellExecutor) RebaseStart(
	ctx context.Context, base, editor string,
) error {
	cmd := exec.CommandContext(
		ctx, "git", "rebase", "-i", base,
	)
	if e.WorkDir != "" {
		cmd.Dir = e.WorkDir
	}

	// Set the custom sequence editor and also set GIT_EDITOR to handle
	// any commit message prompts (e.g., during squash operations).
	// Using "cat" as GIT_EDITOR makes git think the message file was saved
	// unchanged (cat outputs the file and exits 0), accepting the default.
	cmd.Env = append(
		os.Environ(),
		"GIT_SEQUENCE_EDITOR="+editor,
		"GIT_EDITOR=cat",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check if rebase is paused due to conflict (exit code 1).
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				// Rebase paused, not an error.
				return nil
			}
		}

		return fmt.Errorf("rebase failed: %w: %s", err, stderr.String())
	}

	return nil
}

// RebaseStatus returns the current rebase state.
func (e *ShellExecutor) RebaseStatus(ctx context.Context) (*RebaseState, error) {
	gitDir, err := e.gitDir(ctx)
	if err != nil {
		return nil, err
	}

	// Check for rebase-merge directory (interactive rebase).
	rebaseMergeDir := filepath.Join(gitDir, "rebase-merge")
	if _, err := os.Stat(rebaseMergeDir); os.IsNotExist(err) {
		// Also check for rebase-apply (non-interactive).
		rebaseApplyDir := filepath.Join(gitDir, "rebase-apply")
		if _, err := os.Stat(rebaseApplyDir); os.IsNotExist(err) {
			return &RebaseState{
				InProgress: false,
				State:      RebaseStateNone,
			}, nil
		}
	}

	state := &RebaseState{
		InProgress: true,
		State:      RebaseStateNormal,
	}

	// Read original branch name.
	if data, err := os.ReadFile(
		filepath.Join(rebaseMergeDir, "head-name"),
	); err == nil {
		branch := strings.TrimSpace(string(data))
		branch = strings.TrimPrefix(branch, "refs/heads/")
		state.OriginalBranch = branch
	}

	// Read onto reference.
	if data, err := os.ReadFile(
		filepath.Join(rebaseMergeDir, "onto"),
	); err == nil {
		state.OntoRef = strings.TrimSpace(string(data))
	}

	// Count total and remaining from todo files.
	if data, err := os.ReadFile(
		filepath.Join(rebaseMergeDir, "git-rebase-todo"),
	); err == nil {
		state.RemainingCount = countTodoEntries(string(data))
	}

	if data, err := os.ReadFile(
		filepath.Join(rebaseMergeDir, "done"),
	); err == nil {
		state.CompletedCount = countTodoEntries(string(data))
	}

	state.TotalCount = state.RemainingCount + state.CompletedCount

	// Check for conflicts.
	conflicts, err := e.getConflicts(ctx)
	if err == nil && len(conflicts) > 0 {
		state.State = RebaseStateConflict
		state.Conflicts = conflicts
	}

	return state, nil
}

// getConflicts returns a list of files with merge conflicts.
func (e *ShellExecutor) getConflicts(ctx context.Context) ([]ConflictInfo, error) {
	output, err := e.run(ctx, nil, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var conflicts []ConflictInfo

	for _, path := range strings.Split(output, "\n") {
		path = strings.TrimSpace(path)
		if path != "" {
			conflicts = append(conflicts, ConflictInfo{
				Path:         path,
				ConflictType: "content",
			})
		}
	}

	return conflicts, nil
}

// countTodoEntries counts non-comment, non-empty lines in a todo file.
func countTodoEntries(content string) int {
	count := 0

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			count++
		}
	}

	return count
}

// RebaseContinue continues an in-progress rebase.
func (e *ShellExecutor) RebaseContinue(ctx context.Context) error {
	_, err := e.run(ctx, nil, "rebase", "--continue")

	return err
}

// RebaseAbort aborts an in-progress rebase.
func (e *ShellExecutor) RebaseAbort(ctx context.Context) error {
	_, err := e.run(ctx, nil, "rebase", "--abort")

	return err
}

// RebaseSkip skips the current commit during rebase.
func (e *ShellExecutor) RebaseSkip(ctx context.Context) error {
	_, err := e.run(ctx, nil, "rebase", "--skip")

	return err
}

// Compile-time check that ShellExecutor implements Executor.
var _ Executor = (*ShellExecutor)(nil)
