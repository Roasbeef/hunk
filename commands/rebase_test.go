package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/roasbeef/hunk/testutil"
	"github.com/stretchr/testify/require"
)

var (
	hunkBinaryPath string
	buildOnce      sync.Once
	buildErr       error
)

// buildHunkBinary builds the hunk binary once for all tests.
// This is needed because rebase run uses os.Executable() to invoke itself
// as the sequence editor, which doesn't work with the test binary.
func buildHunkBinary(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		tmpDir, err := os.MkdirTemp("", "hunk-test-binary-*")
		if err != nil {
			buildErr = err
			return
		}

		binaryName := "hunk"
		if runtime.GOOS == "windows" {
			binaryName = "hunk.exe"
		}

		hunkBinaryPath = filepath.Join(tmpDir, binaryName)

		cmd := exec.Command("go", "build", "-o", hunkBinaryPath, "./cmd/hunk")
		cmd.Dir = filepath.Join(os.Getenv("GOPATH"), "src/github.com/roasbeef/hunk")

		// If GOPATH is not set, try relative path.
		if _, err := os.Stat(cmd.Dir); os.IsNotExist(err) {
			// We're probably running from within the project.
			cmd.Dir = "."

			// Go up directories to find the project root.
			for range 5 {
				if _, err := os.Stat(filepath.Join(cmd.Dir, "cmd/hunk/main.go")); err == nil {
					break
				}

				cmd.Dir = filepath.Join(cmd.Dir, "..")
			}
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = &exec.ExitError{Stderr: out}
			return
		}
	})

	if buildErr != nil {
		t.Skipf("failed to build hunk binary: %v", buildErr)
	}

	return hunkBinaryPath
}

// runHunkCommand runs the hunk command using the built binary.
func runHunkCommand(t *testing.T, repoDir string, args ...string) (string, error) {
	t.Helper()

	binary := buildHunkBinary(t)

	fullArgs := append([]string{"--dir", repoDir}, args...)
	cmd := exec.Command(binary, fullArgs...)

	out, err := cmd.CombinedOutput()

	return string(out), err
}

func TestRebaseList(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 3 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("feature1.txt", "feature 1\n")
	repo.CommitAll("Feature commit 1")
	hash1 := repo.GetShortHash()

	repo.WriteFile("feature2.txt", "feature 2\n")
	repo.CommitAll("Feature commit 2")
	hash2 := repo.GetShortHash()

	repo.WriteFile("feature3.txt", "feature 3\n")
	repo.CommitAll("Feature commit 3")
	hash3 := repo.GetShortHash()

	// Test text output.
	t.Run("text output", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "rebase", "list", "--onto", "main"})

		var stdout bytes.Buffer
		rootCmd.SetOut(&stdout)

		err := rootCmd.Execute()
		require.NoError(t, err)

		output := stdout.String()
		require.Contains(t, output, "3 commit(s) to rebase")
		require.Contains(t, output, hash1)
		require.Contains(t, output, hash2)
		require.Contains(t, output, hash3)
	})

	// Test JSON output.
	t.Run("json output", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{
			"--dir", repo.Dir, "--json",
			"rebase", "list", "--onto", "main",
		})

		var stdout bytes.Buffer
		rootCmd.SetOut(&stdout)

		err := rootCmd.Execute()
		require.NoError(t, err)

		var output rebaseListOutput
		err = json.Unmarshal(stdout.Bytes(), &output)
		require.NoError(t, err)

		require.Equal(t, "main", output.Base)
		require.Equal(t, 3, output.Count)
		require.Len(t, output.Commits, 3)

		require.Equal(t, hash1, output.Commits[0].ShortHash)
		require.Equal(t, hash2, output.Commits[1].ShortHash)
		require.Equal(t, hash3, output.Commits[2].ShortHash)
	})

	// Test no commits to rebase.
	t.Run("no commits", func(t *testing.T) {
		repo.CheckoutBranch("main")

		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{
			"--dir", repo.Dir,
			"rebase", "list", "--onto", "main",
		})

		var stdout bytes.Buffer
		rootCmd.SetOut(&stdout)

		err := rootCmd.Execute()
		require.NoError(t, err)

		output := stdout.String()
		require.Contains(t, output, "No commits to rebase")
	})
}

func TestRebaseStatusNoRebase(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	repo.WriteFile("file.txt", "content\n")
	repo.CommitAll("Initial commit")

	// Test text output.
	t.Run("text output", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "rebase", "status"})

		var stdout bytes.Buffer
		rootCmd.SetOut(&stdout)

		err := rootCmd.Execute()
		require.NoError(t, err)

		output := stdout.String()
		require.Contains(t, output, "No rebase in progress")
	})

	// Test JSON output.
	t.Run("json output", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "--json", "rebase", "status"})

		var stdout bytes.Buffer
		rootCmd.SetOut(&stdout)

		err := rootCmd.Execute()
		require.NoError(t, err)

		var output rebaseStatusOutput
		err = json.Unmarshal(stdout.Bytes(), &output)
		require.NoError(t, err)

		require.False(t, output.InProgress)
		require.Equal(t, "none", output.State)
	})
}

func TestRebaseRunSimple(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")
	hash2 := repo.GetShortHash()

	// Rebase picking both commits using the binary.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		hash1+","+hash2,
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "completed successfully")

	// Verify both files still exist.
	require.True(t, repo.FileExists("a.txt"))
	require.True(t, repo.FileExists("b.txt"))
}

func TestRebaseRunDrop(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")
	hash2 := repo.GetShortHash()

	initialCount := repo.GetCommitCount()

	// Rebase dropping the second commit using the binary.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1+",drop:"+hash2,
	)
	require.NoError(t, err, "output: %s", output)

	// Should have one fewer commit.
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount-1, finalCount)

	// File a should exist, file b should not.
	require.True(t, repo.FileExists("a.txt"))
	require.False(t, repo.FileExists("b.txt"))
}

func TestRebaseRunSquash(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 3 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("a.txt", "a content updated\n")
	repo.CommitAll("Update file a")
	hash2 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")
	hash3 := repo.GetShortHash()

	initialCount := repo.GetCommitCount()

	// Squash second commit into first using the binary.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1+",fixup:"+hash2+",pick:"+hash3,
	)
	require.NoError(t, err, "output: %s", output)

	// Should have one fewer commit (squash combines two).
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount-1, finalCount)

	// All files should exist.
	require.True(t, repo.FileExists("a.txt"))
	require.True(t, repo.FileExists("b.txt"))

	// File a should have updated content.
	content := repo.ReadFile("a.txt")
	require.Equal(t, "a content updated\n", content)
}

func TestRebaseRunReorder(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("first.txt", "first\n")
	repo.CommitAll("First commit")
	hash1 := repo.GetShortHash()

	repo.WriteFile("second.txt", "second\n")
	repo.CommitAll("Second commit")
	hash2 := repo.GetShortHash()

	// Rebase with reversed order using the binary.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		hash2+","+hash1, // Reversed.
	)
	require.NoError(t, err, "output: %s", output)

	// Both files should exist.
	require.True(t, repo.FileExists("first.txt"))
	require.True(t, repo.FileExists("second.txt"))

	// Check commit order - "First commit" should now be more recent.
	log := repo.LogOneline()
	lines := strings.Split(strings.TrimSpace(log), "\n")
	require.GreaterOrEqual(t, len(lines), 2)

	// Most recent commit should be "First commit".
	require.Contains(t, lines[0], "First commit")
	require.Contains(t, lines[1], "Second commit")
}

func TestRebaseControlCommandsNoRebase(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	repo.WriteFile("file.txt", "content\n")
	repo.CommitAll("Initial commit")

	// Continue should fail when no rebase in progress.
	t.Run("continue", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "rebase", "continue"})

		var stderr bytes.Buffer
		rootCmd.SetErr(&stderr)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no rebase in progress")
	})

	// Abort should fail when no rebase in progress.
	t.Run("abort", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "rebase", "abort"})

		var stderr bytes.Buffer
		rootCmd.SetErr(&stderr)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no rebase in progress")
	})

	// Skip should fail when no rebase in progress.
	t.Run("skip", func(t *testing.T) {
		rootCmd := NewRootCmd()
		rootCmd.SetArgs([]string{"--dir", repo.Dir, "rebase", "skip"})

		var stderr bytes.Buffer
		rootCmd.SetErr(&stderr)

		err := rootCmd.Execute()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no rebase in progress")
	})
}

func TestRebaseRunSquashConcatsMessages(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits (distinct messages).
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("First feature: add file a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Second feature: add file b")
	hash2 := repo.GetShortHash()

	initialCount := repo.GetCommitCount()

	// Squash second commit into first (no custom message - git concatenates).
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1+",squash:"+hash2,
	)
	require.NoError(t, err, "output: %s", output)

	// Should have one fewer commit (squash combines two).
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount-1, finalCount)

	// Get the full commit message.
	fullMessage := repo.Git("log", "-1", "--format=%B")

	// Message should contain text from both original commits.
	require.Contains(t, fullMessage, "First feature")
	require.Contains(t, fullMessage, "Second feature")
}

func TestRebaseRunReword(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Original message for a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")
	hash2 := repo.GetShortHash()

	initialCount := repo.GetCommitCount()

	// Reword action should be accepted and rebase completes.
	// Note: With GIT_EDITOR=cat, the message stays unchanged since the editor
	// just outputs the original. This tests that reword is parsed correctly.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"reword:"+hash1+",pick:"+hash2,
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "completed successfully")

	// Commit count should be unchanged (no squashing).
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount, finalCount)

	// Files should be preserved.
	require.True(t, repo.FileExists("a.txt"))
	require.True(t, repo.FileExists("b.txt"))
	require.Equal(t, "a content\n", repo.ReadFile("a.txt"))

	// Log should still contain the original message (GIT_EDITOR=cat preserves it).
	log := repo.LogOneline()
	require.Contains(t, log, "Original message for a")
}

func TestRebaseRunExec(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 2 commits.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")
	hash1 := repo.GetShortHash()

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")
	hash2 := repo.GetShortHash()

	initialCount := repo.GetCommitCount()

	// Run exec command between picks.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1+",exec:touch marker.txt,pick:"+hash2,
	)
	require.NoError(t, err, "output: %s", output)

	// Commit count should be unchanged.
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount, finalCount)

	// Marker file should exist (exec command ran).
	require.True(t, repo.FileExists("marker.txt"))

	// Both commits should be preserved.
	require.True(t, repo.FileExists("a.txt"))
	require.True(t, repo.FileExists("b.txt"))
}

func TestRebaseRunConflict(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit with shared.txt.
	repo.WriteFile("shared.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with conflicting commits.
	repo.CreateBranch("feature")

	// Commit 1: change to "change A".
	repo.WriteFile("shared.txt", "change A\n")
	repo.CommitAll("Change A")
	hash1 := repo.GetShortHash()

	// Commit 2: change to "change B" (on top of A).
	repo.WriteFile("shared.txt", "change B\n")
	repo.CommitAll("Change B")
	hash2 := repo.GetShortHash()

	// Reorder commits to trigger conflict: pick B first, then A.
	// B expects "change A" as base, but if applied first, base is "base content".
	// Then A expects "base content" but sees "change B".
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash2+",pick:"+hash1,
	)

	// The rebase should pause due to conflicts, not return an error.
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "paused due to conflicts")

	// Verify rebase is in progress.
	require.True(t, repo.FileExists(".git/rebase-merge"))
}

func TestRebaseRunExecNewlineRejected(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with 1 commit.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")
	hash1 := repo.GetShortHash()

	// Try exec command with embedded newline (command injection attempt).
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1+",exec:echo one\npick:inject",
	)

	// Should fail with error about newlines.
	require.Error(t, err)
	require.Contains(t, output, "cannot contain newlines")
}

func TestRebaseRunLongMessage(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with a commit that has a long message.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")

	// Create a commit with a long message (500+ characters).
	longMessage := strings.Repeat("This is a long commit message. ", 17)[:500]
	repo.Git("add", "-A")
	repo.Git("commit", "-m", longMessage)
	hash1 := repo.GetShortHash()

	// Pick the commit with the long message.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "run", "--onto", "main",
		"pick:"+hash1,
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "completed successfully")

	// Verify the long message is preserved.
	fullMessage := repo.Git("log", "-1", "--format=%B")
	require.Contains(t, fullMessage, longMessage[:50])
	require.Contains(t, fullMessage, longMessage[450:])
}

func TestRebaseAutosquashBasic(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with commits and a fixup.
	repo.CreateBranch("feature")

	repo.WriteFile("feature.txt", "feature content\n")
	repo.CommitAll("Add feature")

	repo.WriteFile("other.txt", "other content\n")
	repo.CommitAll("Add other file")

	// Create a fixup commit targeting "Add feature".
	repo.WriteFile("feature.txt", "feature content updated\n")
	repo.Git("add", "-A")
	repo.Git("commit", "-m", "fixup! Add feature")

	initialCount := repo.GetCommitCount()

	// Run autosquash.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "autosquash", "--onto", "main",
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "1 fixup(s) squashed")

	// Should have one fewer commit (fixup was squashed).
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount-1, finalCount)

	// Both files should exist with updated content.
	require.True(t, repo.FileExists("feature.txt"))
	require.True(t, repo.FileExists("other.txt"))
	require.Equal(t, "feature content updated\n", repo.ReadFile("feature.txt"))
}

func TestRebaseAutosquashNoFixups(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with regular commits (no fixups).
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")

	initialCount := repo.GetCommitCount()

	// Run autosquash - should report nothing to do.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "autosquash", "--onto", "main",
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "No fixup/squash commits found")

	// Commit count should be unchanged.
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount, finalCount)
}

func TestRebaseAutosquashDryRun(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with a fixup.
	repo.CreateBranch("feature")

	repo.WriteFile("feature.txt", "feature content\n")
	repo.CommitAll("Add feature")

	repo.WriteFile("feature.txt", "feature updated\n")
	repo.Git("add", "-A")
	repo.Git("commit", "-m", "fixup! Add feature")

	initialCount := repo.GetCommitCount()

	// Run autosquash with --dry-run.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "autosquash", "--onto", "main", "--dry-run",
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "Dry run")
	require.Contains(t, output, "1 fixup")

	// Commit count should be unchanged (dry run).
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount, finalCount)

	// Original content should still be there (not updated).
	require.Equal(t, "feature updated\n", repo.ReadFile("feature.txt"))
}

func TestRebaseAutosquashMultipleFixups(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Create base commit on main.
	repo.WriteFile("base.txt", "base content\n")
	repo.CommitAll("Base commit")

	// Create feature branch with multiple fixups.
	repo.CreateBranch("feature")

	repo.WriteFile("a.txt", "a content\n")
	repo.CommitAll("Add file a")

	repo.WriteFile("b.txt", "b content\n")
	repo.CommitAll("Add file b")

	// Fixup for "Add file a".
	repo.WriteFile("a.txt", "a updated\n")
	repo.Git("add", "-A")
	repo.Git("commit", "-m", "fixup! Add file a")

	// Fixup for "Add file b".
	repo.WriteFile("b.txt", "b updated\n")
	repo.Git("add", "-A")
	repo.Git("commit", "-m", "fixup! Add file b")

	initialCount := repo.GetCommitCount()

	// Run autosquash.
	output, err := runHunkCommand(
		t, repo.Dir,
		"rebase", "autosquash", "--onto", "main",
	)
	require.NoError(t, err, "output: %s", output)
	require.Contains(t, output, "2 fixup(s) squashed")

	// Should have two fewer commits.
	finalCount := repo.GetCommitCount()
	require.Equal(t, initialCount-2, finalCount)

	// Files should have updated content.
	require.Equal(t, "a updated\n", repo.ReadFile("a.txt"))
	require.Equal(t, "b updated\n", repo.ReadFile("b.txt"))
}
