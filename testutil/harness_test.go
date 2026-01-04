package testutil_test

import (
	"testing"

	"github.com/roasbeef/hunk/testutil"
	"github.com/stretchr/testify/require"
)

func TestGitTestRepo(t *testing.T) {
	repo := testutil.NewGitTestRepo(t)

	// Write a file.
	repo.WriteFile("main.go", "package main\n\nfunc main() {}\n")

	// Verify it exists.
	require.True(t, repo.FileExists("main.go"))

	// Read it back.
	content := repo.ReadFile("main.go")
	require.Equal(t, "package main\n\nfunc main() {}\n", content)

	// Commit it.
	repo.CommitAll("initial commit")

	// Make a change.
	repo.WriteFile("main.go", "package main\n\n// Added comment.\nfunc main() {}\n")

	// Get the diff.
	diffOutput := repo.Diff()
	require.Contains(t, diffOutput, "+// Added comment.")
}

func TestComparisonTest(t *testing.T) {
	setup := func(r *testutil.GitTestRepo) {
		r.WriteFile("main.go", "package main\n\nfunc main() {}\n")
		r.CommitAll("initial")
		r.WriteFile("main.go", "package main\n\n// Changed.\nfunc main() {}\n")
	}

	ct := testutil.NewComparisonTest(t, setup)

	// Both repos should have the same unstaged changes.
	ct.AssertSameUnstagedDiff()

	// Stage in both repos.
	ct.Expected.StageFile("main.go")
	ct.Actual.StageFile("main.go")

	// Both should have the same staged changes.
	ct.AssertSameDiff()

	// Both should have the same file content.
	ct.AssertSameContent("main.go")
}
