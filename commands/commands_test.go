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

// TestStagePureAddSubrangeInsideStruct is the end-to-end regression test
// for the silent-misplacement bug originally reported against hunk: when
// new fields are inserted inside an existing Go struct and the user stages
// only a sub-range of the new fields, the staged content must land inside
// the struct body, NOT appended at EOF after `func Helper()`.
//
// The bug shape: a pure-addition group with no context lines between the
// selected sub-range and the surrounding unselected adds emitted a hunk
// with no anchor, which `git apply --cached` silently fuzzed onto the file
// boundary. The patch package now coalesces selections within the same
// pure-add group and walks past unselected additions to grab real anchor
// context, producing a hunk that places the selected lines exactly where
// the user expected them.
func TestStagePureAddSubrangeInsideStruct(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	original := `package foo

type S struct {
	Field1 int
}

func Helper() {}
`
	writeFile(t, dir, "test.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Insert five new fields inside the struct body. The unified diff
	// will show a single pure-add group from new lines 5..9.
	modified := `package foo

type S struct {
	Field1 int
	Field2 int
	Field3 int
	Field4 int
	Field5 int
	Field6 int
}

func Helper() {}
`
	writeFile(t, dir, "test.go", modified)

	// Stage just the middle two — Field3 and Field4 — exactly the
	// shape of the original bug report. Without the fix this either
	// errored out with "patch does not apply" or silently placed the
	// fields after `func Helper()`.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "test.go:6-7"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err,
		"pure-add subrange staging must not error",
	)

	// The staged version of the file must contain Field3 and Field4
	// inside the struct, in the original Field-N ordering. The
	// unselected fields must NOT be staged.
	staged := gitCmd(t, dir, "show", ":test.go")
	expected := `package foo

type S struct {
	Field1 int
	Field3 int
	Field4 int
}

func Helper() {}
`
	require.Equal(t, expected, staged,
		"staged content must place selected fields inside the "+
			"struct, not at EOF",
	)
}

// TestStageNonContiguousAddsInSameGroup is the regression test for the
// secondary symptom: when two non-adjacent selections target lines in the
// SAME pure-add group, the two selections must coalesce into one hunk
// with proper anchor context on both sides. Otherwise the second hunk
// would lack a leading anchor and `git apply` would silently misplace it.
func TestStageNonContiguousAddsInSameGroup(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	original := `package foo

type S struct {
	Field1 int
}

func Helper() {}
`
	writeFile(t, dir, "test.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	modified := fiveFieldStructFile
	writeFile(t, dir, "test.go", modified)

	// Skip Field3 and Field4 — stage only the first and last new
	// fields. With the pre-fix code these were two separate blocks
	// neither of which had complete anchor context.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "test.go:5,8"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err,
		"non-contiguous staging within same pure-add group must "+
			"succeed",
	)

	staged := gitCmd(t, dir, "show", ":test.go")
	expected := `package foo

type S struct {
	Field1 int
	Field2 int
	Field5 int
}

func Helper() {}
`
	require.Equal(t, expected, staged,
		"non-contiguous selection must land adjacently inside the "+
			"struct in selection order",
	)
}

// fiveFieldStructFile is a shared text fixture: a Go file with a struct
// holding five sequentially-named fields. Several pure-add and
// pure-delete subrange tests share this exact shape, so we extract it
// here to avoid stringly-duplicated literals.
const fiveFieldStructFile = `package foo

type S struct {
	Field1 int
	Field2 int
	Field3 int
	Field4 int
	Field5 int
}

func Helper() {}
`

// TestStagePureDeleteSubrangeInsideStruct is the symmetric regression
// test for pure-delete groups: when the user stages a sub-range of
// deletions inside a larger pure-delete group, the patch must correctly
// describe the unselected deletions as context lines so git apply
// accepts it and the staged file retains exactly the unselected lines.
// Without the fix, the unselected deletion wedged between two selected
// ones was dropped from the patch body and git apply rejected the patch
// with "patch does not apply".
func TestStagePureDeleteSubrangeInsideStruct(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	original := fiveFieldStructFile
	writeFile(t, dir, "test.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Remove Field2..Field4. Diff has one pure-delete group of 3 lines.
	modified := `package foo

type S struct {
	Field1 int
	Field5 int
}

func Helper() {}
`
	writeFile(t, dir, "test.go", modified)

	// Stage only Field2 and Field4 (old lines 5 and 7), keeping Field3
	// (old line 6) un-staged inside the group. This is the pure-delete
	// analogue of TestStageNonContiguousAddsInSameGroup.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "test.go:5,7"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err,
		"non-contiguous staging within same pure-delete group "+
			"must succeed",
	)

	staged := gitCmd(t, dir, "show", ":test.go")
	expected := `package foo

type S struct {
	Field1 int
	Field3 int
	Field5 int
}

func Helper() {}
`
	require.Equal(t, expected, staged,
		"pure-delete subrange must remove selected fields and "+
			"preserve the unselected one in between",
	)
}

// TestStagePureDeleteMiddleOnlySingle covers the single-selection-in-
// the-middle-of-a-pure-delete-group case. Selecting only the middle of
// `-B,-C,-D` pre-fix emitted a hunk with no anchor context on either
// side (both neighbours are unselected deletions). With unselected
// deletions now re-tagged as context, the hunk picks up `-B` and `-D`
// as context anchors and the patch applies cleanly.
func TestStagePureDeleteMiddleOnlySingle(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	original := fiveFieldStructFile
	writeFile(t, dir, "test.go", original)
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	modified := `package foo

type S struct {
	Field1 int
	Field5 int
}

func Helper() {}
`
	writeFile(t, dir, "test.go", modified)

	// Stage only Field3 (old line 6) — the middle deletion.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs([]string{"--dir", dir, "stage", "test.go:6"})

	var stdout bytes.Buffer
	rootCmd.SetOut(&stdout)

	err := rootCmd.Execute()
	require.NoError(t, err,
		"single-selection inside a pure-delete group must succeed",
	)

	staged := gitCmd(t, dir, "show", ":test.go")
	expected := `package foo

type S struct {
	Field1 int
	Field2 int
	Field4 int
	Field5 int
}

func Helper() {}
`
	require.Equal(t, expected, staged,
		"only the selected deletion must be applied; the "+
			"surrounding unselected deletions stay in place",
	)
}

// TestStageErrorDoesNotPrintUsage verifies the cobra footgun fix: when the
// stage command's RunE returns an error (e.g., selection doesn't match any
// line), the command must NOT dump its help text. The help dump made real
// failures invisible to scripts and AI agents piping stdout/stderr.
func TestStageErrorDoesNotPrintUsage(t *testing.T) {
	dir, cleanup := setupTestRepo(t)
	defer cleanup()

	writeFile(t, dir, "main.go", "package main\n")
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")

	// Now make a change so there's a diff to operate on.
	writeFile(t, dir, "main.go", "package main\n// added\n")

	// Select a line number with no matching change.
	rootCmd := commands.NewRootCmd()
	rootCmd.SetArgs(
		[]string{"--dir", dir, "stage", "main.go:9999"},
	)

	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)

	err := rootCmd.Execute()
	require.Error(t, err,
		"selection with no matching lines must return an error",
	)

	// Cobra writes the help block (signalled by "Usage:" and the
	// "Examples:" header) on RunE error unless SilenceUsage is set.
	// Neither of these should appear.
	combined := stdout.String() + stderr.String()
	require.NotContains(t, combined, "Usage:",
		"stage error must not print usage block",
	)
	require.NotContains(t, combined, "Examples:",
		"stage error must not print examples block",
	)
}
