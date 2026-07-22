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

// maxContext is the number of anchor context lines emitted on each side of a
// change block, matching git's default unified-diff context. It also bounds
// how close two blocks may sit before their context windows overlap and they
// must be coalesced into a single hunk (see coalesceBlocks).
const maxContext = 3

// changeBlock represents a contiguous group of selected changes within a hunk.
// Indices refer to positions in the original hunk's Lines slice.
type changeBlock struct {
	startIdx int // Index where this block starts (inclusive).
	endIdx   int // Index where this block ends (exclusive).
}

// lineRef describes a single line included in the output hunk. It pairs
// an index into the original hunk's Lines slice with an optional override
// that forces the emitted line to render as a context line regardless of
// its original Op. The override is used for unselected deletions that fall
// inside a coalesced block range or in the context-expansion path: they
// exist in the old file at their recorded position and survive in the new
// file after our partial patch applies, so the patch must describe them
// as context lines to match the actual old-file shape.
type lineRef struct {
	idx       int  // Index into original.Lines.
	asContext bool // If true, render with Op=OpContext.
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

	// Coalesce blocks whose context windows would overlap. Two adjacent
	// blocks each reach up to maxContext lines toward the other; when the
	// gap between them is small enough, both would emit the SAME context
	// lines (one as trailing context, the other as leading context),
	// producing sub-hunks with overlapping old-side ranges that git apply
	// rejects. Merging them into a single block emits the shared context
	// once and yields a valid hunk.
	blocks = coalesceBlocks(hunk, blocks)

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
// may include unselected change lines in the middle — buildHunkFromBlock
// drops unselected additions and re-tags unselected deletions as context.
// This coalescing is what lets a selection like `:5,8` against a single
// pure-add group produce one anchored hunk instead of two hunks neither of
// which has trailing context, and is what lets `:2,4` against a pure-delete
// group produce a single hunk that correctly describes the unselected
// deletion in the middle.
//
// For mixed change groups (containing both additions and deletions), the
// group is treated as atomic: if ANY line in the group is selected, ALL
// lines are force-selected. This prevents invalid patches where some
// deletions in a replacement are removed but their partners are left in
// place.
//
// It also returns the per-line selection mask. A `true` entry on a change
// line means "this change line is selected and should appear in the output
// patch as a real change"; `false` on a change line means "this change
// line lives in the same hunk but is not being staged". Callers use the
// mask to decide how to treat each line of the block range and the
// context-expansion path: unselected additions are dropped, unselected
// deletions are re-tagged as context, and selected lines render unchanged.
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

// coalesceBlocks merges adjacent change blocks whose context windows would
// overlap into single blocks. Each block, when built into its own sub-hunk,
// reaches up to maxContext context lines toward its neighbor; when the number
// of context lines between two blocks is <= 2*maxContext, both sub-hunks would
// claim the same context line(s) — one as trailing context, the other as
// leading context — producing sub-hunks with overlapping old-side ranges that
// `git apply` rejects with "patch does not apply". Merging emits the shared
// context exactly once and mirrors git's own hunk-coalescing rule. Blocks are
// assumed sorted by startIdx, as findChangeBlocks returns them.
func coalesceBlocks(hunk *diff.Hunk, blocks []changeBlock) []changeBlock {
	if len(blocks) <= 1 {
		return blocks
	}

	merged := []changeBlock{blocks[0]}
	for _, next := range blocks[1:] {
		prev := &merged[len(merged)-1]

		gap := gapContextLines(hunk, prev.endIdx, next.startIdx)
		if gap <= 2*maxContext {
			// Extend the previous block to swallow the gap and the
			// next block; collectHunkIndices renders the shared
			// context between them once.
			prev.endIdx = next.endIdx
			continue
		}

		merged = append(merged, next)
	}

	return merged
}

// gapContextLines counts the lines between two blocks that expandContext would
// consume from the context budget: real context lines and unselected deletions
// (re-tagged as context), but NOT unselected additions (which the walk steps
// past at no cost, so they never anchor a sub-hunk and cannot overlap). This
// mirrors expandContext so the coalescing decision reflects the context each
// sub-hunk would actually emit.
func gapContextLines(hunk *diff.Hunk, start, end int) int {
	n := 0
	for i := start; i < end; i++ {
		line := hunk.Lines[i]
		switch {
		case !line.IsChange():
			n++
		case line.Op == diff.OpDelete:
			n++
		}
	}

	return n
}

// buildHunkFromBlock creates a valid hunk from a change block. It walks
// outward from the block, collecting up to maxContext (3) context lines on
// each side. The walk handles unselected change lines asymmetrically:
//
//   - Unselected ADDITIONS are stepped past silently (they live only in
//     the new file, so they cannot appear in a patch describing the old
//     file). Without this, a block of additions wedged inside a larger
//     pure-add group would emit a hunk with no anchor — git apply either
//     rejects ("patch does not apply") or silently fuzzes onto the wrong
//     line (often EOF).
//
//   - Unselected DELETIONS are emitted in the body as CONTEXT lines (Op
//     flipped to OpContext at materialization). They exist in the old
//     file at their recorded positions and survive in the new file after
//     our partial patch applies, so re-tagging them as context produces a
//     body that matches the actual old-file shape. Without this, a
//     selection like `:2,4` against a `-B,-C,-D` group would emit a patch
//     missing -C from the old-side accounting and git apply would reject
//     it.
//
// The walk still STOPS at selected change lines (which belong to other
// blocks) and at context lines beyond the maxContext budget.
func buildHunkFromBlock(
	original *diff.Hunk, block changeBlock, selected []bool,
) *diff.Hunk {
	refs := collectHunkIndices(original, block, selected)
	if len(refs) == 0 {
		return nil
	}

	// Materialize the included lines, applying the asContext override.
	lines := make([]diff.DiffLine, len(refs))
	for i, ref := range refs {
		line := original.Lines[ref.idx]
		if ref.asContext {
			// An unselected deletion re-tagged as context. We keep
			// the original Content and OldLineNum (still valid in
			// the old file); NewLineNum stays at its original 0
			// since the patch text only consults Op and Content.
			// RecalculateLineCounts uses Op so the line is now
			// counted on both the old and new side, matching the
			// fact that the line survives the partial patch.
			line.Op = diff.OpContext
		}
		lines[i] = line
	}

	indices := make([]int, len(refs))
	for i, ref := range refs {
		indices[i] = ref.idx
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

// collectHunkIndices returns the line refs that should appear in the
// output hunk, in original-hunk order. It starts with the selected change
// lines from the block (re-tagging unselected deletions inside the block
// range as context lines and dropping unselected additions), then prepends
// backward-expansion context and appends forward-expansion context. The
// context-expansion walk steps past unselected additions silently and
// captures unselected deletions as context lines (counted against the
// context budget). It stops at selected change lines (those belong to
// other blocks).
func collectHunkIndices(
	original *diff.Hunk, block changeBlock, selected []bool,
) []lineRef {
	// Block lines: real context lines (rare inside a block in
	// practice) and selected change lines render unchanged.
	// Unselected ADDITIONS are dropped entirely (they live only in
	// the new file and we're not staging them). Unselected DELETIONS
	// are re-tagged as context — they exist in the old file at the
	// recorded position and stay in the new file after our partial
	// patch applies.
	var refs []lineRef
	for i := block.startIdx; i < block.endIdx; i++ {
		line := original.Lines[i]
		switch {
		case !line.IsChange():
			refs = append(refs, lineRef{idx: i})
		case selected[i]:
			refs = append(refs, lineRef{idx: i})
		case line.Op == diff.OpDelete:
			refs = append(refs, lineRef{idx: i, asContext: true})
		}
		// Unselected additions inside the block range are dropped.
	}

	backward := expandContext(
		original, selected, block.startIdx-1, -1, maxContext,
	)
	refs = append(backward, refs...)

	forward := expandContext(
		original, selected, block.endIdx, +1, maxContext,
	)
	refs = append(refs, forward...)

	return refs
}

// expandContext walks original.Lines starting at `start`, stepping by
// `step` (-1 for backward, +1 for forward), and returns up to maxContext
// line refs to use as anchor context. Real context lines are captured
// as-is. Unselected additions are silently stepped past and do NOT count
// against the budget (they live only in the new file). Unselected
// deletions are captured as context lines (asContext=true) and DO count
// against the budget — they exist in the old file and re-tagging them as
// context describes their presence accurately. Any other non-context
// encounter (selected change line belonging to another block, end of
// hunk) stops the walk. For backward walks the returned slice is in
// ascending order so it can be prepended verbatim.
func expandContext(
	original *diff.Hunk, selected []bool, start, step, limit int,
) []lineRef {
	var out []lineRef
	i := start
	prepend := func(ref lineRef) {
		if step < 0 {
			out = append([]lineRef{ref}, out...)
		} else {
			out = append(out, ref)
		}
	}
	for len(out) < limit {
		if i < 0 || i >= len(original.Lines) {
			break
		}
		line := original.Lines[i]
		switch {
		case line.Op == diff.OpContext:
			prepend(lineRef{idx: i})
		case line.Op == diff.OpAdd && !selected[i]:
			// Walk past — additions live only in the new file
			// and do not consume the context budget.
		case line.Op == diff.OpDelete && !selected[i]:
			// Re-tag the unselected deletion as a context line.
			// It exists in the old file at this position and
			// survives our partial patch, so this is the correct
			// old-side description.
			prepend(lineRef{idx: i, asContext: true})
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

// computeNewStart returns the NewStart for the output hunk. When the
// first emitted line has a stable old-side anchor (a context line or a
// deletion), we use its OldLineNum: for a single-hunk patch this is the
// staged-file position at apply time (the staged file equals the old
// file before any of our hunks have run). For multi-hunk patches that
// emit multiple sub-hunks against the same file, the NewStart of later
// sub-hunks may be off relative to the post-earlier-hunks staged file —
// but git apply re-anchors on context content, so this is a hint rather
// than a hard constraint. The fallback walks back through the ORIGINAL
// hunk to find a line with a non-zero NewLineNum when the sub-hunk opens
// with an addition.
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
