// Package git provides an abstraction layer for git operations.
// This enables testing without actual git repositories.
package git

import (
	"context"
	"io"
	"time"
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

	// RebaseList returns commits that would be rebased onto the given base.
	RebaseList(ctx context.Context, base string) ([]CommitInfo, error)

	// RebaseStart begins an interactive rebase with a custom sequence editor.
	// The editor command is invoked by git to modify the todo file.
	RebaseStart(ctx context.Context, base, editor string) error

	// RebaseStatus returns the current rebase state.
	RebaseStatus(ctx context.Context) (*RebaseState, error)

	// RebaseContinue continues an in-progress rebase.
	RebaseContinue(ctx context.Context) error

	// RebaseAbort aborts an in-progress rebase.
	RebaseAbort(ctx context.Context) error

	// RebaseSkip skips the current commit during rebase.
	RebaseSkip(ctx context.Context) error
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

// CommitInfo contains metadata about a commit.
type CommitInfo struct {
	// Hash is the full commit hash.
	Hash string

	// ShortHash is the abbreviated commit hash (7 characters).
	ShortHash string

	// Subject is the first line of the commit message.
	Subject string

	// Author is the commit author in "Name <email>" format.
	Author string

	// Date is when the commit was authored.
	Date time.Time
}

// RebaseStateType indicates the current state of a rebase operation.
type RebaseStateType string

const (
	// RebaseStateNone indicates no rebase is in progress.
	RebaseStateNone RebaseStateType = "none"

	// RebaseStateNormal indicates rebase is progressing normally.
	RebaseStateNormal RebaseStateType = "normal"

	// RebaseStateConflict indicates rebase has stopped due to conflicts.
	RebaseStateConflict RebaseStateType = "conflict"

	// RebaseStateEdit indicates rebase has stopped for commit editing.
	RebaseStateEdit RebaseStateType = "edit"
)

// RebaseState represents the current state of an interactive rebase.
type RebaseState struct {
	// InProgress is true if a rebase operation is active.
	InProgress bool

	// State indicates the current rebase state.
	State RebaseStateType

	// CurrentCommit is the commit currently being rebased (if any).
	CurrentCommit *CommitInfo

	// CurrentAction is the action being performed (pick, squash, etc.).
	CurrentAction string

	// TotalCount is the total number of commits to rebase.
	TotalCount int

	// RemainingCount is the number of commits remaining.
	RemainingCount int

	// CompletedCount is the number of commits already rebased.
	CompletedCount int

	// Conflicts lists any files with conflicts.
	Conflicts []ConflictInfo

	// OriginalBranch is the branch being rebased.
	OriginalBranch string

	// OntoRef is the target base reference.
	OntoRef string
}

// ConflictInfo describes a file with merge conflicts.
type ConflictInfo struct {
	// Path is the file path relative to repo root.
	Path string

	// ConflictType describes the type of conflict (content, delete, etc.).
	ConflictType string
}
