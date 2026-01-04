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
// Context lines are preserved as needed for valid patches.
func filterHunks(hunks []*diff.Hunk, sel *diff.FileSelection) []*diff.Hunk {
	var result []*diff.Hunk

	for _, hunk := range hunks {
		filtered := filterHunk(hunk, sel)
		if filtered != nil {
			result = append(result, filtered)
		}
	}

	return result
}

// filterHunk filters a single hunk based on selection.
func filterHunk(hunk *diff.Hunk, sel *diff.FileSelection) *diff.Hunk {
	var lines []diff.DiffLine
	hasChanges := false

	for _, line := range hunk.Lines {
		lineNum := effectiveLineNum(line)

		if line.Op == diff.OpContext {
			// Keep context lines (will be trimmed later if not needed).
			lines = append(lines, line)
		} else if sel.Contains(lineNum) {
			// Selected change line.
			lines = append(lines, line)
			hasChanges = true
		}
		// Unselected change lines are converted to context.
		// This maintains line number integrity.
	}

	if !hasChanges {
		return nil
	}

	// Build new hunk with proper context.
	return buildFilteredHunk(hunk, lines)
}

// effectiveLineNum returns the line number to use for selection matching.
// For additions, uses NewLineNum. For deletions, uses OldLineNum.
func effectiveLineNum(line diff.DiffLine) int {
	if line.Op == diff.OpAdd {
		return line.NewLineNum
	}

	return line.OldLineNum
}

// buildFilteredHunk constructs a new hunk with trimmed context.
func buildFilteredHunk(
	original *diff.Hunk, lines []diff.DiffLine,
) *diff.Hunk {

	const maxContext = 3

	// Find first and last change indices.
	firstChange := -1
	lastChange := -1

	for i, line := range lines {
		if line.IsChange() {
			if firstChange == -1 {
				firstChange = i
			}
			lastChange = i
		}
	}

	if firstChange == -1 {
		return nil
	}

	// Calculate context bounds.
	startIdx := firstChange - maxContext
	if startIdx < 0 {
		startIdx = 0
	}

	endIdx := lastChange + maxContext + 1
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	// Slice to keep only needed lines.
	lines = lines[startIdx:endIdx]

	// Reconstruct the hunk with correct line numbers.
	result := &diff.Hunk{
		Section: original.Section,
		Lines:   lines,
	}

	// Set start line numbers from first line.
	if len(lines) > 0 {
		first := lines[0]
		if first.OldLineNum > 0 {
			result.OldStart = first.OldLineNum
		} else {
			// For additions at start, use previous context if available.
			result.OldStart = original.OldStart
		}

		if first.NewLineNum > 0 {
			result.NewStart = first.NewLineNum
		} else {
			result.NewStart = original.NewStart
		}
	}

	result.RecalculateLineCounts()

	return result
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
