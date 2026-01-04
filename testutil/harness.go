// Package testutil provides test helpers for git repository testing.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// GitTestRepo creates a temporary git repository for testing.
type GitTestRepo struct {
	t   *testing.T
	Dir string
}

// NewGitTestRepo creates a new test repo with git initialized.
func NewGitTestRepo(t *testing.T) *GitTestRepo {
	t.Helper()

	dir, err := os.MkdirTemp("", "hunk-test-*")
	require.NoError(t, err)

	repo := &GitTestRepo{t: t, Dir: dir}
	t.Cleanup(repo.cleanup)

	// Initialize git repo with basic config.
	repo.Git("init")
	repo.Git("config", "user.email", "test@test.com")
	repo.Git("config", "user.name", "Test User")

	return repo
}

func (r *GitTestRepo) cleanup() {
	os.RemoveAll(r.Dir)
}

// Git runs a git command in the test repo.
func (r *GitTestRepo) Git(args ...string) string {
	r.t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}

	return string(out)
}

// GitMayFail runs a git command that may fail, returning the error.
func (r *GitTestRepo) GitMayFail(args ...string) (string, error) {
	r.t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()

	return string(out), err
}

// WriteFile creates or overwrites a file in the repo.
func (r *GitTestRepo) WriteFile(path, content string) {
	r.t.Helper()

	fullPath := filepath.Join(r.Dir, path)
	dir := filepath.Dir(fullPath)

	err := os.MkdirAll(dir, 0755)
	require.NoError(r.t, err)

	err = os.WriteFile(fullPath, []byte(content), 0644)
	require.NoError(r.t, err)
}

// ReadFile reads a file from the repo.
func (r *GitTestRepo) ReadFile(path string) string {
	r.t.Helper()

	data, err := os.ReadFile(filepath.Join(r.Dir, path))
	require.NoError(r.t, err)

	return string(data)
}

// FileExists checks if a file exists in the repo.
func (r *GitTestRepo) FileExists(path string) bool {
	r.t.Helper()

	_, err := os.Stat(filepath.Join(r.Dir, path))

	return err == nil
}

// CommitAll stages and commits all changes.
func (r *GitTestRepo) CommitAll(msg string) {
	r.t.Helper()

	r.Git("add", "-A")
	r.Git("commit", "-m", msg)
}

// StageFile stages a specific file.
func (r *GitTestRepo) StageFile(path string) {
	r.t.Helper()

	r.Git("add", path)
}

// Diff returns the current unstaged diff.
func (r *GitTestRepo) Diff() string {
	r.t.Helper()

	return r.Git("diff", "--no-color")
}

// DiffCached returns the current staged diff.
func (r *GitTestRepo) DiffCached() string {
	r.t.Helper()

	return r.Git("diff", "--cached", "--no-color")
}

// ComparisonTest represents a comparison between two git operations.
// Used to verify hunk produces identical results to manual git operations.
type ComparisonTest struct {
	t        *testing.T
	Expected *GitTestRepo
	Actual   *GitTestRepo
}

// NewComparisonTest creates two identical repos for comparison testing.
// The setup function is called on both repos to establish identical state.
func NewComparisonTest(
	t *testing.T, setup func(r *GitTestRepo),
) *ComparisonTest {

	t.Helper()

	expected := NewGitTestRepo(t)
	actual := NewGitTestRepo(t)

	setup(expected)
	setup(actual)

	return &ComparisonTest{
		t:        t,
		Expected: expected,
		Actual:   actual,
	}
}

// AssertSameContent verifies both repos have identical file contents.
func (c *ComparisonTest) AssertSameContent(paths ...string) {
	c.t.Helper()

	for _, path := range paths {
		exp := c.Expected.ReadFile(path)
		act := c.Actual.ReadFile(path)

		require.Equal(c.t, exp, act,
			"file %s differs between expected and actual", path)
	}
}

// AssertSameDiff verifies both repos have identical staged diffs.
func (c *ComparisonTest) AssertSameDiff() {
	c.t.Helper()

	expDiff := c.Expected.DiffCached()
	actDiff := c.Actual.DiffCached()

	require.Equal(c.t, expDiff, actDiff,
		"staged diffs differ between expected and actual")
}

// AssertSameUnstagedDiff verifies both repos have identical unstaged diffs.
func (c *ComparisonTest) AssertSameUnstagedDiff() {
	c.t.Helper()

	expDiff := c.Expected.Diff()
	actDiff := c.Actual.Diff()

	require.Equal(c.t, expDiff, actDiff,
		"unstaged diffs differ between expected and actual")
}
