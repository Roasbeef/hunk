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

// TestProperty_PureDeleteSubrangeIsAlwaysAnchored is the symmetric
// integration property for pure-delete groups. It builds a struct file
// with N pre-existing fields, deletes ALL of them in the modified file
// (creating one pure-delete group of size N), then picks a random
// non-empty subset of those deletions to STAGE. The generated patch is
// piped through real `git apply --cached`; afterward the staged file
// must equal the original file minus exactly the selected fields, with
// the unselected fields preserved verbatim in their original positions.
//
// This catches:
//   - Pure-delete subranges that drop unselected deletions from the
//     body and produce an invalid patch git apply rejects.
//   - Single-deletion-in-the-middle hunks with no anchor context that
//     git apply cannot place.
//   - Off-by-one position errors in the generated `@@` headers.
func TestProperty_PureDeleteSubrangeIsAlwaysAnchored(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numFields := rapid.IntRange(2, 12).Draw(rt, "numFields")

		// Build the original file: struct with numFields pre-existing
		// fields. Each field name is unique so we can identify it.
		var origLines []string
		origLines = append(origLines,
			"package foo",
			"",
			"type S struct {",
		)
		var fieldNames []string
		for i := range numFields {
			fieldNames = append(
				fieldNames, fmt.Sprintf("Field%d", i),
			)
			origLines = append(origLines,
				"\t"+fieldNames[i]+" int",
			)
		}
		origLines = append(origLines,
			"}",
			"",
			"func Helper() {}",
		)

		// Modified file: drop every field. The diff has a single
		// pure-delete group of length numFields.
		var modLines []string
		modLines = append(modLines, origLines[:3]...)
		modLines = append(modLines, origLines[3+numFields:]...)

		// Random non-empty subset of fields to STAGE for deletion.
		stageMask := rapid.SliceOfN(
			rapid.Bool(), numFields, numFields,
		).Draw(rt, "stageMask")
		anyStaged := false
		for _, b := range stageMask {
			if b {
				anyStaged = true
				break
			}
		}
		if !anyStaged {
			stageMask[0] = true
		}

		// In the OLD file, fields occupy lines 4..3+numFields.
		var stagedLineNums []int
		for i, sel := range stageMask {
			if sel {
				stagedLineNums = append(stagedLineNums, 4+i)
			}
		}

		repoDir := newPropertyRepo(rt)
		origPath := filepath.Join(repoDir, "test.go")
		writeAll(rt, origPath, origLines)
		runGit(rt, repoDir, "add", "test.go")
		runGit(rt, repoDir, "commit", "-q", "-m", "init")

		writeAll(rt, origPath, modLines)

		diffText := runGit(rt, repoDir, "diff", "--no-color")
		parsed, err := diff.Parse(diffText)
		require.NoError(rt, err)

		var parts []string
		for _, ln := range stagedLineNums {
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
		require.NotEmpty(rt, patchBytes)

		if err := applyCached(repoDir, patchBytes); err != nil {
			rt.Fatalf(
				"git apply --cached rejected patch: %v\n"+
					"Patch:\n%s",
				err, patchBytes,
			)
		}

		// Build the expected staged file: original minus the
		// staged fields, with unstaged fields preserved in place.
		var expected []string
		expected = append(expected, origLines[:3]...)
		for i, sel := range stageMask {
			if !sel {
				expected = append(expected,
					"\t"+fieldNames[i]+" int",
				)
			}
		}
		expected = append(expected, origLines[3+numFields:]...)

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

// TestProperty_MultiGroupSelectionAlwaysApplies is the property safeguarding
// the "partial-hunk staging emits non-applying patches" bug (reported against
// v1.0.2). It builds a file containing several mixed replacement groups
// separated by a RANDOM number of context lines — deliberately straddling the
// 2*maxContext coalescing boundary so some neighbouring groups share context
// and must coalesce while others stay split. It then stages a random subset
// of whole groups and asserts two invariants:
//
//   - the generated patch applies cleanly via `git apply --cached` (the bug
//     emitted sub-hunks with overlapping old-side ranges git rejected), and
//   - the resulting index content equals the original with EXACTLY the
//     selected groups transformed to their new form and every unselected
//     group left untouched.
//
// Selecting whole groups sidesteps the mixed-group atomicity axis (covered
// elsewhere) so this property isolates cross-group context coalescing.
func TestProperty_MultiGroupSelectionAlwaysApplies(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		numGroups := rapid.IntRange(2, 4).Draw(rt, "numGroups")

		// A group is a mixed replacement: delCount old lines removed,
		// addCount new lines added. gapCount context lines follow the
		// group (except after the last). Gaps range across the
		// 2*maxContext=6 boundary so both the coalesce and split paths
		// are exercised within a single file.
		type group struct {
			delCount, addCount, gapAfter int
			intended                     bool  // Chosen for staging.
			oldNums, newNums             []int // 1-indexed positions.
		}

		groups := make([]group, numGroups)
		anyIntended := false
		for i := range groups {
			groups[i] = group{
				delCount: rapid.IntRange(1, 4).
					Draw(rt, fmt.Sprintf("del%d", i)),
				addCount: rapid.IntRange(1, 4).
					Draw(rt, fmt.Sprintf("add%d", i)),
				gapAfter: rapid.IntRange(1, 8).
					Draw(rt, fmt.Sprintf("gap%d", i)),
				intended: rapid.Bool().
					Draw(rt, fmt.Sprintf("sel%d", i)),
			}
			anyIntended = anyIntended || groups[i].intended
		}
		if !anyIntended {
			groups[0].intended = true
		}

		// Build the original and modified files line by line, recording
		// each group's old-file and new-file line numbers so we can (a)
		// form the selection string and (b) resolve which groups the
		// tool will actually stage. The FILE:LINES syntax matches
		// additions by new-line number and deletions by old-line
		// number in one shared integer space, so a selection integer
		// aimed at one group's addition can also land on another
		// group's deletion. We resolve the ACTUAL selection below and
		// build the expectation from it, keeping this property focused
		// on patch validity and coalescing rather than selection
		// parsing.
		head := []string{"package foo", ""}

		origLines := append([]string{}, head...)
		modLines := append([]string{}, head...)

		for i := range groups {
			g := &groups[i]
			for j := 0; j < g.delCount; j++ {
				origLines = append(
					origLines,
					fmt.Sprintf("g%d_old_%d", i, j),
				)
				g.oldNums = append(g.oldNums, len(origLines))
			}
			for j := 0; j < g.addCount; j++ {
				modLines = append(
					modLines,
					fmt.Sprintf("g%d_new_%d", i, j),
				)
				g.newNums = append(g.newNums, len(modLines))
			}
			if i < len(groups)-1 {
				for j := 0; j < g.gapAfter; j++ {
					ctx := fmt.Sprintf("ctx_%d_%d", i, j)
					origLines = append(origLines, ctx)
					modLines = append(modLines, ctx)
				}
			}
		}
		tail := "func Tail() {}"
		origLines = append(origLines, tail)
		modLines = append(modLines, tail)

		// The selection set: every intended group's added new-line
		// numbers. New-line numbers are unique per line, so this hits
		// exactly the intended additions — but the same integers may
		// also match some unintended group's deletions by old-line.
		selSet := make(map[int]bool)
		var selectedNewLines []int
		for i := range groups {
			if !groups[i].intended {
				continue
			}
			for _, ln := range groups[i].newNums {
				selSet[ln] = true
				selectedNewLines = append(selectedNewLines, ln)
			}
		}

		// Resolve the ACTUAL selection: a mixed group is staged in full
		// if ANY of its additions (by new-line) or deletions (by
		// old-line) is in the selection set. Build the expected index
		// from that resolution.
		hit := func(nums []int) bool {
			for _, n := range nums {
				if selSet[n] {
					return true
				}
			}
			return false
		}

		expected := append([]string{}, head...)
		for i := range groups {
			g := &groups[i]
			staged := hit(g.newNums) || hit(g.oldNums)
			if staged {
				for j := 0; j < g.addCount; j++ {
					expected = append(
						expected,
						fmt.Sprintf("g%d_new_%d", i, j),
					)
				}
			} else {
				for j := 0; j < g.delCount; j++ {
					expected = append(
						expected,
						fmt.Sprintf("g%d_old_%d", i, j),
					)
				}
			}
			if i < len(groups)-1 {
				for j := 0; j < g.gapAfter; j++ {
					expected = append(
						expected,
						fmt.Sprintf("ctx_%d_%d", i, j),
					)
				}
			}
		}
		expected = append(expected, tail)

		repoDir := newPropertyRepo(rt)
		origPath := filepath.Join(repoDir, "test.go")
		writeAll(rt, origPath, origLines)
		runGit(rt, repoDir, "add", "test.go")
		runGit(rt, repoDir, "commit", "-q", "-m", "init")

		writeAll(rt, origPath, modLines)

		diffText := runGit(rt, repoDir, "diff", "--no-color")
		parsed, err := diff.Parse(diffText)
		require.NoError(rt, err)

		var parts []string
		for _, ln := range selectedNewLines {
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
		require.NotEmpty(rt, patchBytes)

		if err := applyCached(repoDir, patchBytes); err != nil {
			rt.Fatalf(
				"git apply --cached rejected patch: %v\n"+
					"Patch:\n%s",
				err, patchBytes,
			)
		}

		got := readStagedFile(rt, repoDir, "test.go")
		require.Equal(
			rt, strings.Join(expected, "\n")+"\n", got,
			"staged index mismatch.\nPatch:\n%s", patchBytes,
		)
	})
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
