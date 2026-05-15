// Package patch provides functionality for generating patches from selections.
package patch

import (
	"bytes"
	"fmt"

	"github.com/roasbeef/hunk/diff"
)

// Generate creates a patch containing only the selected lines.
// The patch can be applied with `git apply --cached`.
func Generate(
	parsed *diff.ParsedDiff, selections []*diff.FileSelection,
) ([]byte, error) {
	// Build a map for fast lookup.
	selMap := diff.NewSelectionMap(selections)

	var buf bytes.Buffer

	for file := range parsed.Files() {
		sel := selMap.Get(file.Path())
		if sel == nil {
			// Try both old and new names.
			sel = selMap.Get(file.OldName)
			if sel == nil {
				sel = selMap.Get(file.NewName)
			}
		}

		if sel == nil {
			continue
		}

		// Filter hunks to only include selected lines.
		filteredHunks := filterHunks(file.Hunks, sel)
		if len(filteredHunks) == 0 {
			continue
		}

		// Write file header.
		fmt.Fprintf(&buf, "--- a/%s\n", file.OldName)
		fmt.Fprintf(&buf, "+++ b/%s\n", file.NewName)

		// Write hunks.
		for _, hunk := range filteredHunks {
			buf.WriteString(hunk.Header())
			buf.WriteByte('\n')

			for _, line := range hunk.Lines {
				buf.WriteString(line.String())
				buf.WriteByte('\n')
			}
		}
	}

	return buf.Bytes(), nil
}

// filterHunks returns hunks containing only the selected lines.
// Context lines are preserved as needed for valid patches. When non-contiguous
// lines are selected within a hunk, the hunk is split into multiple hunks.
func filterHunks(hunks []*diff.Hunk, sel *diff.FileSelection) []*diff.Hunk {
	var result []*diff.Hunk

	for _, hunk := range hunks {
		filtered := filterHunk(hunk, sel)
		result = append(result, filtered...)
	}

	return result
}

// changeBlock represents a contiguous group of selected changes within a hunk.
// Indices refer to positions in the original hunk's Lines slice.
type changeBlock struct {
	startIdx int // Index where this block starts (inclusive).
	endIdx   int // Index where this block ends (exclusive).
}

// filterHunk filters a single hunk based on selection. When non-contiguous
// changes are selected, the hunk is split into multiple hunks, one for each
// contiguous block of selected changes. Each resulting hunk is independently
// valid for git apply.
func filterHunk(hunk *diff.Hunk, sel *diff.FileSelection) []*diff.Hunk {
	// Find contiguous blocks of selected changes plus the per-line
	// selection mask. The mask lets buildHunkFromBlock distinguish
	// "selected change we own" from "unselected change we must walk
	// past when reaching for anchor context".
	blocks, selected := findChangeBlocks(hunk, sel)
	if len(blocks) == 0 {
		return nil
	}

	// Build a separate hunk for each block.
	var result []*diff.Hunk
	for _, block := range blocks {
		h := buildHunkFromBlock(hunk, block, selected)
		if h != nil {
			result = append(result, h)
		}
	}

	return result
}

// findChangeBlocks identifies contiguous blocks of selected changes within a
// hunk. Each contiguous run of change lines (no context between them) forms
// a "group"; within a group, all selected lines are coalesced into a single
// block spanning from the first selected line to the last. The block range
// may include unselected change lines in the middle — those are filtered
// out at patch-emission time. This coalescing is what lets a selection like
// `:5,8` against a single pure-add group produce one anchored hunk instead
// of two hunks neither of which has trailing context.
//
// For mixed change groups (containing both additions and deletions), the
// group is treated as atomic: if ANY line in the group is selected, ALL
// lines are force-selected. This prevents invalid patches where some
// deletions in a replacement are removed but their partners are left in
// place.
//
// It also returns the per-line selection mask. A `true` entry on a change
// line means "this change line is selected and should appear in the output
// patch"; `false` on a change line means "this change line lives in the
// same hunk but is not being staged". Callers use the mask to filter the
// block's range and to walk past unselected additions when expanding
// anchor context around a block.
func findChangeBlocks(hunk *diff.Hunk, sel *diff.FileSelection) (
	[]changeBlock, []bool,
) {
	// Build a selected-line set that expands mixed change groups.
	// A mixed group is a contiguous run of change lines containing
	// both additions and deletions. When any member is selected,
	// all members are force-selected.
	selected := make([]bool, len(hunk.Lines))

	// First pass: mark individually selected lines and identify
	// mixed change groups.
	type groupSpan struct {
		start, end  int  // Indices into hunk.Lines (end exclusive).
		hasAdd      bool // Group contains at least one addition.
		hasDel      bool // Group contains at least one deletion.
		anySelected bool // Group has at least one selected line.
	}

	var groups []groupSpan
	var cur *groupSpan

	for i, line := range hunk.Lines {
		if !line.IsChange() {
			if cur != nil {
				groups = append(groups, *cur)
				cur = nil
			}
			continue
		}

		if cur == nil {
			cur = &groupSpan{start: i}
		}
		cur.end = i + 1

		switch line.Op {
		case diff.OpAdd:
			cur.hasAdd = true
		case diff.OpDelete:
			cur.hasDel = true
		}

		lineNum := effectiveLineNum(line)
		if sel.Contains(lineNum) {
			selected[i] = true
			cur.anySelected = true
		}
	}
	if cur != nil {
		groups = append(groups, *cur)
	}

	// Expand mixed groups: if a group has both adds and deletes and
	// any member is selected, force-select all members.
	for _, g := range groups {
		if g.hasAdd && g.hasDel && g.anySelected {
			for i := g.start; i < g.end; i++ {
				selected[i] = true
			}
		}
	}

	// Second pass: emit one block per group, spanning from the group's
	// first selected line to its last selected line (inclusive of any
	// unselected change lines in between, which buildHunkFromBlock
	// will filter out). Skip groups with no selected lines.
	var blocks []changeBlock
	for _, g := range groups {
		first := -1
		last := -1
		for i := g.start; i < g.end; i++ {
			if selected[i] {
				if first == -1 {
					first = i
				}
				last = i
			}
		}
		if first == -1 {
			continue
		}
		blocks = append(blocks, changeBlock{
			startIdx: first,
			endIdx:   last + 1,
		})
	}

	return blocks, selected
}

// buildHunkFromBlock creates a valid hunk from a change block. It walks
// outward from the block, collecting up to maxContext (3) context lines on
// each side. The walk steps past UNSELECTED ADDITIONS — additions that exist
// only in the new file and therefore cannot appear in the patch — to reach
// real anchor context in the old file. Without this, a block consisting of
// addition lines wedged inside a larger pure-add group would emit a hunk
// with no anchor, which `git apply` either rejects ("patch does not apply")
// or, worse, silently fuzzes onto the wrong line (often EOF).
//
// The walk does NOT step past unselected DELETIONS, selected change lines
// (which belong to other blocks), or context lines beyond the maxContext
// budget. Unselected deletions exist in the old file at their recorded
// positions, so skipping over them would break the old-side line accounting
// of the resulting patch.
func buildHunkFromBlock(
	original *diff.Hunk, block changeBlock, selected []bool,
) *diff.Hunk {
	indices := collectHunkIndices(original, block, selected)
	if len(indices) == 0 {
		return nil
	}

	// Materialize the included lines.
	lines := make([]diff.DiffLine, len(indices))
	for i, idx := range indices {
		lines[i] = original.Lines[idx]
	}

	result := &diff.Hunk{
		Section: original.Section,
		Lines:   lines,
	}

	result.OldStart = computeOldStart(original, indices)
	result.NewStart = computeNewStart(original, indices)
	result.RecalculateLineCounts()

	return result
}

// collectHunkIndices returns the indices into original.Lines that should
// appear in the output hunk, in original-hunk order. It starts with the
// selected change lines from the block, then prepends backward-expansion
// context and appends forward-expansion context. The expansion walks past
// UNSELECTED ADDITIONS — additions that exist only in the new file and
// therefore cannot anchor a patch — but stops at selected change lines
// (they belong to other blocks) and at unselected deletions (they exist
// in the old file at recorded positions and dropping them would break the
// old-side line accounting).
func collectHunkIndices(
	original *diff.Hunk, block changeBlock, selected []bool,
) []int {
	const maxContext = 3

	// Block lines: context lines (rare inside a block in practice)
	// plus only the SELECTED change lines. Unselected change lines
	// inside the block range come from the same pure-add or pure-delete
	// group as the surrounding selected lines; the patch silently drops
	// them so the staged file contains exactly what was selected.
	var indices []int
	for i := block.startIdx; i < block.endIdx; i++ {
		line := original.Lines[i]
		if !line.IsChange() || selected[i] {
			indices = append(indices, i)
		}
	}

	backward := expandContext(
		original, selected, block.startIdx-1, -1, maxContext,
	)
	indices = append(backward, indices...)

	forward := expandContext(
		original, selected, block.endIdx, +1, maxContext,
	)
	indices = append(indices, forward...)

	return indices
}

// expandContext walks original.Lines starting at `start`, stepping by
// `step` (-1 for backward, +1 for forward), and returns up to maxContext
// context-line indices encountered. It skips unselected additions and
// stops at any other non-context line. For backward walks the returned
// slice is in ascending order so it can be prepended verbatim.
func expandContext(
	original *diff.Hunk, selected []bool, start, step, limit int,
) []int {
	var out []int
	i := start
	for len(out) < limit {
		if i < 0 || i >= len(original.Lines) {
			break
		}
		line := original.Lines[i]
		switch {
		case line.Op == diff.OpContext:
			if step < 0 {
				out = append([]int{i}, out...)
			} else {
				out = append(out, i)
			}
		case line.Op == diff.OpAdd && !selected[i]:
			// Walk past — additions live only in the new file.
		default:
			return out
		}
		i += step
	}
	return out
}

// computeOldStart returns the OldStart for the output hunk. When the first
// included line is a context or deletion line, its OldLineNum is the
// anchor; when it's an addition we walk back through the ORIGINAL hunk to
// find the most recent line that does have an old-side position, with the
// insertion landing just after it.
func computeOldStart(original *diff.Hunk, indices []int) int {
	first := original.Lines[indices[0]]
	if first.OldLineNum > 0 {
		return first.OldLineNum
	}
	for i := indices[0] - 1; i >= 0; i-- {
		if original.Lines[i].OldLineNum > 0 {
			return original.Lines[i].OldLineNum + 1
		}
	}
	return original.OldStart
}

// computeNewStart returns the NewStart for the output hunk. Within a
// single hunk (before any later hunks shift staged-file positions), the
// first line lands at the same staged-file position as its old-file
// position — so we use OldLineNum when the first line has a stable
// old-side anchor (context line or deletion). Falls back to the original
// NewLineNum when the hunk opens with an addition.
func computeNewStart(original *diff.Hunk, indices []int) int {
	first := original.Lines[indices[0]]
	switch {
	case first.Op == diff.OpContext && first.OldLineNum > 0:
		return first.OldLineNum
	case first.Op == diff.OpDelete && first.OldLineNum > 0:
		return first.OldLineNum
	case first.NewLineNum > 0:
		return first.NewLineNum
	}
	for i := indices[0] - 1; i >= 0; i-- {
		if original.Lines[i].NewLineNum > 0 {
			return original.Lines[i].NewLineNum + 1
		}
	}
	return original.NewStart
}

// effectiveLineNum returns the line number to use for selection matching.
// For additions, uses NewLineNum. For deletions, uses OldLineNum.
func effectiveLineNum(line diff.DiffLine) int {
	if line.Op == diff.OpAdd {
		return line.NewLineNum
	}

	return line.OldLineNum
}

// GenerateForFile creates a patch for a single file with all its changes.
func GenerateForFile(file *diff.FileDiff) []byte {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "--- a/%s\n", file.OldName)
	fmt.Fprintf(&buf, "+++ b/%s\n", file.NewName)

	for _, hunk := range file.Hunks {
		buf.WriteString(hunk.Header())
		buf.WriteByte('\n')

		for _, line := range hunk.Lines {
			buf.WriteString(line.String())
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes()
}

// GenerateForHunk creates a patch for a single hunk.
func GenerateForHunk(file *diff.FileDiff, hunk *diff.Hunk) []byte {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "--- a/%s\n", file.OldName)
	fmt.Fprintf(&buf, "+++ b/%s\n", file.NewName)

	buf.WriteString(hunk.Header())
	buf.WriteByte('\n')

	for _, line := range hunk.Lines {
		buf.WriteString(line.String())
		buf.WriteByte('\n')
	}

	return buf.Bytes()
}
