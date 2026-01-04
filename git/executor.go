package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
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

// Compile-time check that ShellExecutor implements Executor.
var _ Executor = (*ShellExecutor)(nil)
