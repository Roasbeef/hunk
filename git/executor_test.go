package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roasbeef/hunk/git"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "git-executor-test-*")
	require.NoError(t, err)

	cleanup := func() {
		os.RemoveAll(dir)
	}

	// Initialize git repo.
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "user.name", "Test User")

	return dir, cleanup
}

func TestNewShellExecutor(t *testing.T) {
	executor := git.NewShellExecutor("/tmp")
	require.NotNil(t, executor)
	require.Equal(t, "/tmp", executor.WorkDir)

	executor = git.NewShellExecutor("")
	require.NotNil(t, executor)
	require.Empty(t, executor.WorkDir)
}

func TestShellExecutorDiff(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Make changes.
	writeFile(t, dir, "main.go", "package main\n\n// Added.\nfunc main() {}\n")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Get diff.
	diffText, err := executor.Diff(ctx)
	require.NoError(t, err)
	require.Contains(t, diffText, "+// Added.")

	// Diff for specific file.
	diffText, err = executor.Diff(ctx, "main.go")
	require.NoError(t, err)
	require.Contains(t, diffText, "+// Added.")

	// Diff for non-existent file - git returns empty or error depending on version.
	// We just verify no panic.
	_, _ = executor.Diff(ctx, "nonexistent.go")
}

func TestShellExecutorDiffCached(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Make and stage changes.
	writeFile(t, dir, "main.go", "package main\n\n// Staged.\nfunc main() {}\n")
	gitCmd(t, dir, "add", "main.go")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Get cached diff.
	diffText, err := executor.DiffCached(ctx)
	require.NoError(t, err)
	require.Contains(t, diffText, "+// Staged.")
}

func TestShellExecutorApplyPatch(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Create a patch.
	patch := `--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main

+// Added via patch.
 func main() {}
`

	// Apply patch.
	err := executor.ApplyPatch(ctx, strings.NewReader(patch))
	require.NoError(t, err)

	// Verify it's staged.
	diffText, err := executor.DiffCached(ctx)
	require.NoError(t, err)
	require.Contains(t, diffText, "+// Added via patch.")
}

func TestShellExecutorCommit(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and stage a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Commit.
	err := executor.Commit(ctx, "test commit")
	require.NoError(t, err)

	// Verify commit exists.
	log := gitCmd(t, dir, "log", "--oneline")
	require.Contains(t, log, "test commit")
}

func TestShellExecutorReset(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Stage changes.
	writeFile(t, dir, "main.go", "package main\n\n// Changed.\nfunc main() {}\n")
	gitCmd(t, dir, "add", "main.go")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Verify staged.
	diffText, err := executor.DiffCached(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, diffText)

	// Reset.
	err = executor.Reset(ctx)
	require.NoError(t, err)

	// Verify unstaged.
	diffText, err = executor.DiffCached(ctx)
	require.NoError(t, err)
	require.Empty(t, diffText)
}

func TestShellExecutorResetPath(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit files.
	writeFile(t, dir, "a.go", "package a\n")
	writeFile(t, dir, "b.go", "package b\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Stage changes to both.
	writeFile(t, dir, "a.go", "package a\n// changed\n")
	writeFile(t, dir, "b.go", "package b\n// changed\n")
	gitCmd(t, dir, "add", "-A")

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Reset just a.go.
	err := executor.ResetPath(ctx, "a.go")
	require.NoError(t, err)

	// Verify a.go is unstaged but b.go is still staged.
	diffText, err := executor.DiffCached(ctx)
	require.NoError(t, err)
	require.NotContains(t, diffText, "a.go")
	require.Contains(t, diffText, "b.go")
}

func TestShellExecutorStatus(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	executor := git.NewShellExecutor(dir)
	ctx := context.Background()

	// Test with clean repo (just init, no files).
	status, err := executor.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, status)

	// Add untracked file.
	writeFile(t, dir, "untracked.go", "package a\n")

	status, err = executor.Status(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, status.UntrackedFiles, "should have untracked files")

	// Stage the file.
	gitCmd(t, dir, "add", "untracked.go")

	status, err = executor.Status(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, status.StagedFiles, "should have staged files")

	// Commit and then modify.
	gitCmd(t, dir, "commit", "-m", "add file")
	writeFile(t, dir, "untracked.go", "package a\n// modified\n")

	status, err = executor.Status(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, status.UnstagedFiles, "should have unstaged files")
}

func TestShellExecutorRoot(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create subdirectory.
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))

	executor := git.NewShellExecutor(subdir)
	ctx := context.Background()

	root, err := executor.Root(ctx)
	require.NoError(t, err)

	// Resolve symlinks for comparison (macOS /var -> /private/var).
	expectedDir, _ := filepath.EvalSymlinks(dir)
	actualRoot, _ := filepath.EvalSymlinks(root)
	require.Equal(t, expectedDir, actualRoot)
}

func TestShellExecutorErrorHandling(t *testing.T) {
	// Non-existent directory.
	executor := git.NewShellExecutor("/nonexistent/path/that/does/not/exist")
	ctx := context.Background()

	_, err := executor.Diff(ctx)
	require.Error(t, err)
}

// Helper functions.

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()

	// Handle init specially to set default branch.
	if args[0] == "init" {
		args = append([]string{"-c", "init.defaultBranch=main"}, args...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("git %v failed: %v\n%s", args, err, out)
	}

	return string(out)
}
