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
	// Find contiguous blocks of selected changes.
	blocks := findChangeBlocks(hunk, sel)
	if len(blocks) == 0 {
		return nil
	}

	// Build a separate hunk for each block.
	var result []*diff.Hunk
	for _, block := range blocks {
		h := buildHunkFromBlock(hunk, block)
		if h != nil {
			result = append(result, h)
		}
	}

	return result
}

// findChangeBlocks identifies contiguous blocks of selected changes within a
// hunk. Context lines do not break contiguity - only unselected change lines
// create block boundaries.
func findChangeBlocks(hunk *diff.Hunk, sel *diff.FileSelection) []changeBlock {
	var blocks []changeBlock
	var currentBlock *changeBlock

	for i, line := range hunk.Lines {
		if !line.IsChange() {
			// Context lines don't affect block boundaries.
			continue
		}

		lineNum := effectiveLineNum(line)
		isSelected := sel.Contains(lineNum)

		if isSelected {
			if currentBlock == nil {
				// Start a new block.
				currentBlock = &changeBlock{startIdx: i}
			}
			// Extend block to include this line.
			currentBlock.endIdx = i + 1
		} else if currentBlock != nil {
			// Unselected change line breaks the current block.
			blocks = append(blocks, *currentBlock)
			currentBlock = nil
		}
	}

	// Don't forget to close the final block.
	if currentBlock != nil {
		blocks = append(blocks, *currentBlock)
	}

	return blocks
}

// buildHunkFromBlock creates a valid hunk from a change block. It includes
// up to maxContext (3) lines of context before and after the block, stopping
// at unselected change lines.
func buildHunkFromBlock(original *diff.Hunk, block changeBlock) *diff.Hunk {
	const maxContext = 3

	// Expand backward to include context lines.
	startIdx := block.startIdx
	contextBefore := 0
	for i := block.startIdx - 1; i >= 0 && contextBefore < maxContext; i-- {
		if original.Lines[i].Op == diff.OpContext {
			startIdx = i
			contextBefore++
		} else {
			// Hit an unselected change line, stop expanding.
			break
		}
	}

	// Expand forward to include context lines.
	endIdx := block.endIdx
	contextAfter := 0
	for i := block.endIdx; i < len(original.Lines) && contextAfter < maxContext; i++ {
		if original.Lines[i].Op == diff.OpContext {
			endIdx = i + 1
			contextAfter++
		} else {
			// Hit an unselected change line, stop expanding.
			break
		}
	}

	// Copy the selected lines.
	lines := make([]diff.DiffLine, endIdx-startIdx)
	copy(lines, original.Lines[startIdx:endIdx])

	result := &diff.Hunk{
		Section: original.Section,
		Lines:   lines,
	}

	// Set start line numbers from the first line.
	if len(lines) > 0 {
		first := lines[0]
		if first.OldLineNum > 0 {
			result.OldStart = first.OldLineNum
		} else {
			// Addition at start - find a previous line's old number.
			for i := startIdx - 1; i >= 0; i-- {
				if original.Lines[i].OldLineNum > 0 {
					result.OldStart = original.Lines[i].OldLineNum + 1
					break
				}
			}
			if result.OldStart == 0 {
				result.OldStart = original.OldStart
			}
		}

		if first.NewLineNum > 0 {
			result.NewStart = first.NewLineNum
		} else {
			// Deletion at start - find a previous line's new number.
			for i := startIdx - 1; i >= 0; i-- {
				if original.Lines[i].NewLineNum > 0 {
					result.NewStart = original.Lines[i].NewLineNum + 1
					break
				}
			}
			if result.NewStart == 0 {
				result.NewStart = original.NewStart
			}
		}
	}

	result.RecalculateLineCounts()

	return result
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
