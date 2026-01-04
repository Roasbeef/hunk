// Package git provides an abstraction layer for git operations.
// This enables testing without actual git repositories.
package git

import (
	"context"
	"io"
)

// Executor abstracts git operations for testability.
type Executor interface {
	// Diff returns the unified diff for unstaged changes.
	// If paths is non-empty, limits to those paths.
	Diff(ctx context.Context, paths ...string) (string, error)

	// DiffCached returns the unified diff for staged changes.
	DiffCached(ctx context.Context, paths ...string) (string, error)

	// ApplyPatch applies a patch to the staging area.
	// The patch is read from the provided reader.
	ApplyPatch(ctx context.Context, patch io.Reader) error

	// Commit creates a commit with the given message.
	Commit(ctx context.Context, message string) error

	// Reset unstages all staged changes.
	Reset(ctx context.Context) error

	// ResetPath unstages changes for a specific path.
	ResetPath(ctx context.Context, path string) error

	// Status returns the current repository status.
	Status(ctx context.Context) (*RepoStatus, error)

	// Root returns the repository root directory.
	Root(ctx context.Context) (string, error)
}

// RepoStatus represents the current state of the repository.
type RepoStatus struct {
	// StagedFiles lists files with staged changes.
	StagedFiles []string

	// UnstagedFiles lists files with unstaged changes.
	UnstagedFiles []string

	// UntrackedFiles lists untracked files.
	UntrackedFiles []string
}

// FileStatus represents the status of a single file.
type FileStatus struct {
	// Path is the file path relative to repo root.
	Path string

	// Staged indicates if the file has staged changes.
	Staged bool

	// Unstaged indicates if the file has unstaged changes.
	Unstaged bool

	// Untracked indicates if the file is untracked.
	Untracked bool
}
