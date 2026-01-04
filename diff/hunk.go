package diff

import (
	"fmt"
	"iter"
)

// Hunk represents a contiguous block of changes in a file.
type Hunk struct {
	// OldStart is the starting line in the original file.
	OldStart int

	// OldLines is the number of lines from the original file.
	OldLines int

	// NewStart is the starting line in the new file.
	NewStart int

	// NewLines is the number of lines in the new file.
	NewLines int

	// Section is the optional section header (e.g., function name).
	Section string

	// Lines contains all lines in this hunk.
	Lines []DiffLine
}

// Header returns the hunk header in unified diff format.
func (h *Hunk) Header() string {
	header := fmt.Sprintf(
		"@@ -%d,%d +%d,%d @@",
		h.OldStart, h.OldLines, h.NewStart, h.NewLines,
	)

	if h.Section != "" {
		header += " " + h.Section
	}

	return header
}

// All returns an iterator over all lines in this hunk.
func (h *Hunk) All() iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		for _, line := range h.Lines {
			if !yield(line) {
				return
			}
		}
	}
}

// Changes returns an iterator over only changed lines (add/delete).
func (h *Hunk) Changes() iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		for _, line := range h.Lines {
			if line.Op == OpContext {
				continue
			}
			if !yield(line) {
				return
			}
		}
	}
}

// Additions returns an iterator over only added lines.
func (h *Hunk) Additions() iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		for _, line := range h.Lines {
			if line.Op != OpAdd {
				continue
			}
			if !yield(line) {
				return
			}
		}
	}
}

// Deletions returns an iterator over only deleted lines.
func (h *Hunk) Deletions() iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		for _, line := range h.Lines {
			if line.Op != OpDelete {
				continue
			}
			if !yield(line) {
				return
			}
		}
	}
}

// Stats returns addition and deletion counts.
func (h *Hunk) Stats() (added, deleted int) {
	for _, line := range h.Lines {
		switch line.Op {
		case OpAdd:
			added++
		case OpDelete:
			deleted++
		}
	}

	return added, deleted
}

// CanSplit returns true if this hunk can be split into smaller hunks.
// A hunk can be split if there are context lines between change groups.
func (h *Hunk) CanSplit() bool {
	inChange := false
	hasGap := false

	for _, line := range h.Lines {
		if line.Op == OpContext {
			if inChange {
				hasGap = true
			}
		} else {
			if hasGap {
				return true
			}
			inChange = true
		}
	}

	return false
}

// ContainsLine checks if any change in this hunk affects the given line.
// Uses NewLineNum for additions, OldLineNum for deletions.
func (h *Hunk) ContainsLine(lineNum int) bool {
	for _, line := range h.Lines {
		if !line.IsChange() {
			continue
		}

		effectiveLine := line.NewLineNum
		if line.Op == OpDelete {
			effectiveLine = line.OldLineNum
		}

		if effectiveLine == lineNum {
			return true
		}
	}

	return false
}

// ContainsRange checks if any change in this hunk falls within the range.
func (h *Hunk) ContainsRange(start, end int) bool {
	for _, line := range h.Lines {
		if !line.IsChange() {
			continue
		}

		effectiveLine := line.NewLineNum
		if line.Op == OpDelete {
			effectiveLine = line.OldLineNum
		}

		if effectiveLine >= start && effectiveLine <= end {
			return true
		}
	}

	return false
}

// RecalculateLineCounts updates OldLines and NewLines based on Lines slice.
func (h *Hunk) RecalculateLineCounts() {
	h.OldLines = 0
	h.NewLines = 0

	for _, line := range h.Lines {
		switch line.Op {
		case OpContext:
			h.OldLines++
			h.NewLines++
		case OpAdd:
			h.NewLines++
		case OpDelete:
			h.OldLines++
		}
	}
}
