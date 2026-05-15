package patch_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/patch"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// TestProperty_PureAddSubrangeIsAlwaysAnchored is the main property
// safeguarding the silent-misplacement bug: for any pure-add group inside a
// file and any non-empty subset of its added lines, the generated patch
// must apply cleanly via `git apply --cached` AND land the selected lines
// at the expected position (inside the surrounding context, not at EOF).
//
// The rapid generator builds a synthetic "Go struct"-shaped file by
// rendering some fixed leading context, a contiguous run of newly-added
// fields, and some fixed trailing context. It then picks a random subset
// of the new fields to stage, asks the patch package to generate a patch,
// pipes that through real git, and asserts byte-equality of the resulting
// staged-file content against an independently-constructed expectation.
//
// This catches:
//   - Pure-add subranges that emit no anchor and silently land at EOF.
//   - Non-contiguous selections inside one pure-add group that fail to
//     coalesce into a single anchored hunk.
//   - Off-by-one position errors in the generated `@@` headers.
func TestProperty_PureAddSubrangeIsAlwaysAnchored(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numAdded := rapid.IntRange(2, 12).Draw(rt, "numAdded")

		// Build an original file with a struct body and a helper
		// function. The struct has one existing field which serves
		// as anchor context.
		var origLines = []string{
			"package foo",
			"",
			"type S struct {",
			"\tField1 int",
			"}",
			"",
			"func Helper() {}",
		}

		// Build the modified file: insert numAdded new fields between
		// Field1 (orig line 4) and the closing brace (orig line 5).
		// Each new field gets a unique name so we can detect which
		// ones the patch actually stages.
		var addedNames []string
		for i := range numAdded {
			addedNames = append(
				addedNames, fmt.Sprintf("Extra%d", i),
			)
		}

		var modLines []string
		modLines = append(modLines, origLines[:4]...)
		for _, n := range addedNames {
			modLines = append(modLines, "\t"+n+" int")
		}
		modLines = append(modLines, origLines[4:]...)

		// Pick a random non-empty subset of the added fields to
		// stage. selected[i] == true means addedNames[i] gets staged.
		selectedMask := rapid.SliceOfN(
			rapid.Bool(), numAdded, numAdded,
		).Draw(rt, "selectedMask")
		anySelected := false
		for _, b := range selectedMask {
			if b {
				anySelected = true
				break
			}
		}
		if !anySelected {
			selectedMask[0] = true
		}

		// Compute selected line numbers in the NEW file. The new
		// fields occupy lines 5..5+numAdded-1.
		var selectedLines []int
		var expectedStaged []string
		for i, sel := range selectedMask {
			if !sel {
				continue
			}
			selectedLines = append(selectedLines, 5+i)
			expectedStaged = append(
				expectedStaged, "\t"+addedNames[i]+" int",
			)
		}

		// Sandbox: a fresh temp git repo per iteration. This keeps
		// the property test hermetic and means a failure can be
		// reproduced verbatim from the rapid shrunk inputs.
		repoDir := newPropertyRepo(rt)
		origPath := filepath.Join(repoDir, "test.go")
		writeAll(rt, origPath, origLines)
		runGit(rt, repoDir, "add", "test.go")
		runGit(rt, repoDir, "commit", "-q", "-m", "init")

		writeAll(rt, origPath, modLines)

		diffText := runGit(rt, repoDir, "diff", "--no-color")
		parsed, err := diff.Parse(diffText)
		require.NoError(rt, err)

		// Translate selected lines into a FileSelection. The rapid
		// shrinker prefers small ranges so we emit a comma list
		// rather than coalesced ranges.
		var parts []string
		for _, ln := range selectedLines {
			parts = append(parts, fmt.Sprintf("%d", ln))
		}
		sel, err := diff.ParseFileSelection(
			"test.go:" + strings.Join(parts, ","),
		)
		require.NoError(rt, err)

		patchBytes, err := patch.Generate(
			parsed, []*diff.FileSelection{sel},
		)
		require.NoError(rt, err)
		require.NotEmpty(rt, patchBytes,
			"non-empty selection must produce non-empty patch")

		// Apply via real git to surface anchor failures. The patch
		// MUST apply cleanly; "patch does not apply" or silent fuzz
		// onto EOF are both bug signatures.
		if err := applyCached(repoDir, patchBytes); err != nil {
			rt.Fatalf(
				"git apply --cached rejected patch: %v\n"+
					"Patch:\n%s",
				err, patchBytes,
			)
		}

		// Assert the staged file equals original + exactly the
		// selected fields inserted in order between Field1 and the
		// closing brace. If the lines landed at EOF or in some
		// fuzzed-but-applied position, this comparison fails loudly.
		var expected []string
		expected = append(expected, origLines[:4]...)
		expected = append(expected, expectedStaged...)
		expected = append(expected, origLines[4:]...)
		got := readStagedFile(rt, repoDir, "test.go")
		require.Equal(rt, strings.Join(expected, "\n")+"\n", got,
			"staged file content mismatch.\nPatch:\n%s",
			patchBytes,
		)
	})
}

// TestProperty_PatchHunksAlwaysAnchored verifies a purely-textual property
// on the patches hunk emits: every emitted hunk has at least one context
// line either immediately before the first addition OR immediately after
// the last addition. This is necessary for `git apply` to anchor the patch
// without falling back to file-boundary heuristics.
//
// Unlike the integration property above, this one constructs the diff text
// directly so it stays fast and shrinks well.
func TestProperty_PatchHunksAlwaysAnchored(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate a diff with N pure-add lines wedged inside a
		// fixed-shape context block.
		numAdds := rapid.IntRange(1, 8).Draw(rt, "numAdds")
		var diffBuf strings.Builder
		diffBuf.WriteString("--- a/f.go\n")
		diffBuf.WriteString("+++ b/f.go\n")
		fmt.Fprintf(
			&diffBuf, "@@ -1,4 +1,%d @@\n", 4+numAdds,
		)
		diffBuf.WriteString(" line1\n")
		diffBuf.WriteString(" line2\n")
		diffBuf.WriteString(" line3\n")
		for i := range numAdds {
			fmt.Fprintf(&diffBuf, "+add%d\n", i)
		}
		diffBuf.WriteString(" line4\n")

		parsed, err := diff.Parse(diffBuf.String())
		require.NoError(rt, err)

		// Pick a random non-empty subset of new-file line numbers
		// corresponding to the added lines (lines 4..3+numAdds).
		mask := rapid.SliceOfN(
			rapid.Bool(), numAdds, numAdds,
		).Draw(rt, "mask")
		anySelected := false
		for _, b := range mask {
			if b {
				anySelected = true
				break
			}
		}
		if !anySelected {
			mask[0] = true
		}

		var parts []string
		for i, sel := range mask {
			if sel {
				parts = append(parts,
					fmt.Sprintf("%d", 4+i),
				)
			}
		}
		fsel, err := diff.ParseFileSelection(
			"f.go:" + strings.Join(parts, ","),
		)
		require.NoError(rt, err)

		out, err := patch.Generate(
			parsed, []*diff.FileSelection{fsel},
		)
		require.NoError(rt, err)
		require.NotEmpty(rt, out)

		// Inspect each emitted hunk and assert it has an anchor:
		// at least one context line either before the first `+`
		// or after the last `+`.
		hunks := splitHunks(string(out))
		require.NotEmpty(rt, hunks,
			"patch produced no hunks despite non-empty selection",
		)
		for i, h := range hunks {
			lead, trail := countAnchorContext("@@\n" + h)
			if lead == 0 && trail == 0 {
				rt.Fatalf(
					"hunk %d has no anchor context.\n"+
						"Patch:\n%s",
					i, out,
				)
			}
		}
	})
}

// splitHunks breaks a multi-hunk patch body into the body of each hunk,
// discarding the file headers and the `@@` lines themselves. Returns the
// lines of each hunk (one string per hunk, joined with newlines).
func splitHunks(patchText string) []string {
	lines := strings.Split(patchText, "\n")
	var hunks []string
	var cur []string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "--- "),
			strings.HasPrefix(line, "+++ "):
			continue
		case strings.HasPrefix(line, "@@"):
			if len(cur) > 0 {
				hunks = append(hunks, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		hunks = append(hunks, strings.Join(cur, "\n"))
	}
	return hunks
}

// newPropertyRepo creates a fresh temp git repo for one property
// iteration. rapid.T does not expose the standard *testing.T cleanup hook
// so we mop up via t.Cleanup on the underlying t via the rapid harness's
// t.Helper-friendly wrappers. Because rapid runs many iterations in one
// test invocation, leaving repos on disk would balloon disk usage; we
// remove the dir at the end of each iteration explicitly.
func newPropertyRepo(t *rapid.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "hunk-property-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func writeAll(t *rapid.T, path string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func runGit(t *rapid.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}

func applyCached(dir string, patchBytes []byte) error {
	cmd := exec.Command("git", "apply", "--cached", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(string(patchBytes))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

func readStagedFile(t *rapid.T, dir, path string) string {
	t.Helper()
	out := runGit(t, dir, "show", ":"+path)
	return out
}
