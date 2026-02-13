package commands_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/roasbeef/hunk/commands"
	"github.com/stretchr/testify/require"
)

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "commands-test-*")
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

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()

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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
}

func TestNewRootCmd(t *testing.T) {
	cmd := commands.NewRootCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "hunk", cmd.Use)

	// Verify subcommands are registered.
	subCmds := cmd.Commands()
	require.NotEmpty(t, subCmds)

	// Check for expected commands.
	cmdNames := make(map[string]bool)
	for _, c := range subCmds {
		cmdNames[c.Name()] = true
	}

	require.True(t, cmdNames["diff"])
	require.True(t, cmdNames["stage"])
	require.True(t, cmdNames["preview"])
	require.True(t, cmdNames["commit"])
	require.True(t, cmdNames["reset"])
	require.True(t, cmdNames["apply-patch"])
}

func TestNewDiffCmd(t *testing.T) {
	cmd := commands.NewDiffCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "diff [files...]", cmd.Use)
	require.NotEmpty(t, cmd.Short)
	require.NotEmpty(t, cmd.Long)
	require.NotEmpty(t, cmd.Example)
}

func TestNewStageCmd(t *testing.T) {
	cmd := commands.NewStageCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "stage FILE:LINES [FILE:LINES...]", cmd.Use)
	require.NotEmpty(t, cmd.Short)
}

func TestNewPreviewCmd(t *testing.T) {
	cmd := commands.NewPreviewCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "preview", cmd.Use)
}

func TestNewCommitCmd(t *testing.T) {
	cmd := commands.NewCommitCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "commit", cmd.Use)
}

func TestNewResetCmd(t *testing.T) {
	cmd := commands.NewResetCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "reset [files...]", cmd.Use)
}

func TestNewApplyPatchCmd(t *testing.T) {
	cmd := commands.NewApplyPatchCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "apply-patch [file]", cmd.Use)
}

func TestDiffCommandExecution(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Make changes.
	writeFile(t, dir, "main.go", "package main\n\n// Added.\nfunc main() {}\n")

	// Create the command and run it.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "+// Added.")
}

func TestDiffCommandJSON(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Make changes.
	writeFile(t, dir, "main.go", "package main\n\n// Added.\nfunc main() {}\n")

	// Run with JSON flag.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "--json", "diff"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "\"files\"")
}

func TestPreviewCommandEmpty(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file so we have a valid repo.
	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "preview"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "Nothing staged")
}

func TestResetCommand(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file.
	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Stage changes.
	writeFile(t, dir, "main.go", "package main\n// changed\n")
	gitCmd(t, dir, "add", "main.go")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "reset"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "Unstaged")
}

func TestStageCommandInvalidSelection(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "invalid"})

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestCommitCommandNoMessage(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "commit"})

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestConfigDefaults(t *testing.T) {
	// Default config should have empty WorkDir and JSONOut false.
	cfg := commands.Config{}
	require.Empty(t, cfg.WorkDir)
	require.False(t, cfg.JSONOut)
}

func TestDiffCommandNoChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create and commit a file - no uncommitted changes.
	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)
	// Empty diff should succeed without output.
}

func TestApplyPatchCommandNoFile(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Try to apply non-existent file.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "apply-patch", "nonexistent.patch"})

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestDiffCommandStaged(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Stage some changes.
	writeFile(t, dir, "main.go", "package main\n// staged\n")
	gitCmd(t, dir, "add", "main.go")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff", "--staged"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err)

	output := stdout.String()
	require.Contains(t, output, "+// staged")
}

func TestStageCommandNoChanges(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// No unstaged changes.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "main.go:1-10"})

	err := rootCmd.Execute()
	require.Error(t, err) // Should error: no unstaged changes.
}

func TestCommitCommandNothingStaged(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Nothing staged.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "commit", "-m", "test"})

	err := rootCmd.Execute()
	require.Error(t, err) // Should error: nothing staged.
}

func TestDiffCommandFlags(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n// changed\n")

	// Test --files flag - just verify it doesn't error.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff", "--files"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestPreviewCommandRaw(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Stage changes.
	writeFile(t, dir, "main.go", "package main\n// staged\n")
	gitCmd(t, dir, "add", "main.go")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "preview", "--raw"})

	// Just verify no error.
	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestDiffCommandSummary(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n// changed\n")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff", "--summary"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestDiffCommandStageHints(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n// changed\n")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff", "--stage-hints"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

func TestDiffCommandRaw(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	writeFile(t, dir, "main.go", "package main\n// changed\n")

	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "diff", "--raw"})

	err := rootCmd.Execute()
	require.NoError(t, err)
}

// TestStageAtomicReplacementGroup verifies that staging a partial selection
// of a replacement group (mixed deletions + additions) includes the entire
// group. This is the integration test for the "atomic change group" fix
// that prevents "patch does not apply" errors when a user's line range
// boundary falls in the middle of a contiguous replacement.
func TestStageAtomicReplacementGroup(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Original file with multiple functions.
	original := `package main

func oldHelper1() {}
func oldHelper2() {}
func oldHelper3() {}
func oldHelper4() {}

func main() {}
`
	writeFile(t, dir, "main.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Modified file: replace the 4 old helpers with 2 new ones.
	modified := `package main

func newHelper1() {}
func newHelper2() {}

func main() {}
`
	writeFile(t, dir, "main.go", modified)

	// Stage only new line 3 (first addition). The replacement group
	// includes old lines 3-6 (deletions) and new lines 3-4 (additions).
	// Without the atomic group fix, only the addition at line 3 would
	// be staged, creating an invalid patch.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "main.go:3"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err, "stage should succeed for partial "+
		"replacement selection")

	// Verify the staged diff includes all deletions and additions.
	cached := gitCmd(t, dir, "diff", "--cached")
	require.Contains(t, cached, "-func oldHelper1()")
	require.Contains(t, cached, "-func oldHelper2()")
	require.Contains(t, cached, "-func oldHelper3()")
	require.Contains(t, cached, "-func oldHelper4()")
	require.Contains(t, cached, "+func newHelper1()")
	require.Contains(t, cached, "+func newHelper2()")
}

// TestStageMultiHunkReplacementBoundary tests the real-world scenario where
// a non-contiguous selection spans multiple hunks and a range boundary falls
// inside a replacement group in one of the hunks.
func TestStageMultiHunkReplacementBoundary(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Original file with two sections separated by enough context
	// to create separate hunks.
	original := `package main

// Section A.
func a1() {}
func a2() {}

// Separator line 1.
// Separator line 2.
// Separator line 3.
// Separator line 4.
// Separator line 5.

// Section B.
func b1() {}
func b2() {}
func b3() {}

func main() {}
`
	writeFile(t, dir, "main.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Replace both sections.
	modified := `package main

// Section A.
func newA() {}

// Separator line 1.
// Separator line 2.
// Separator line 3.
// Separator line 4.
// Separator line 5.

// Section B.
func newB() {}

func main() {}
`
	writeFile(t, dir, "main.go", modified)

	// Stage only section A changes (new line 4). This should pick up
	// both the deletions (a1, a2) and the addition (newA) in hunk 1,
	// but NOT the section B changes in hunk 2.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "main.go:4"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err, "multi-hunk partial staging should succeed")

	cached := gitCmd(t, dir, "diff", "--cached")
	// Section A changes should be staged.
	require.Contains(t, cached, "-func a1()")
	require.Contains(t, cached, "-func a2()")
	require.Contains(t, cached, "+func newA()")

	// Section B changes should NOT be staged (different hunk).
	require.NotContains(t, cached, "-func b1()")
	require.NotContains(t, cached, "+func newB()")
}
