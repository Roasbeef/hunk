package diff

import (
	"fmt"
	"iter"
	"strings"
)

// FileDiff represents all changes to a single file.
type FileDiff struct {
	// OldName is the path of the original file (with a/ prefix stripped).
	OldName string

	// NewName is the path of the new file (with b/ prefix stripped).
	NewName string

	// Hunks contains all hunks in this file diff.
	Hunks []*Hunk

	// IsBinary is true if this is a binary file.
	IsBinary bool

	// IsNew is true if this is a new file.
	IsNew bool

	// IsDeleted is true if this file is being deleted.
	IsDeleted bool

	// IsRenamed is true if this file was renamed.
	IsRenamed bool
}

// Path returns the canonical file path.
// Uses NewName for additions, OldName for deletions, NewName otherwise.
func (f *FileDiff) Path() string {
	if f.IsDeleted {
		return f.OldName
	}

	return f.NewName
}

// AllHunks returns an iterator over all hunks with their indices.
func (f *FileDiff) AllHunks() iter.Seq2[int, *Hunk] {
	return func(yield func(int, *Hunk) bool) {
		for i, hunk := range f.Hunks {
			if !yield(i, hunk) {
				return
			}
		}
	}
}

// AllLines returns an iterator over all lines across all hunks.
// Yields (hunk index, line) pairs.
func (f *FileDiff) AllLines() iter.Seq2[int, DiffLine] {
	return func(yield func(int, DiffLine) bool) {
		for i, hunk := range f.Hunks {
			for _, line := range hunk.Lines {
				if !yield(i, line) {
					return
				}
			}
		}
	}
}

// AllChanges returns an iterator over only changed lines across all hunks.
func (f *FileDiff) AllChanges() iter.Seq2[int, DiffLine] {
	return func(yield func(int, DiffLine) bool) {
		for i, hunk := range f.Hunks {
			for _, line := range hunk.Lines {
				if line.Op == OpContext {
					continue
				}
				if !yield(i, line) {
					return
				}
			}
		}
	}
}

// Stats returns total addition and deletion counts across all hunks.
func (f *FileDiff) Stats() (added, deleted int) {
	for _, hunk := range f.Hunks {
		a, d := hunk.Stats()
		added += a
		deleted += d
	}

	return added, deleted
}

// HunkContainingLine finds the hunk containing a change at the given line.
// Returns nil if no hunk contains a change at that line.
func (f *FileDiff) HunkContainingLine(lineNum int) *Hunk {
	for _, hunk := range f.Hunks {
		if hunk.ContainsLine(lineNum) {
			return hunk
		}
	}

	return nil
}

// HunksInRange returns all hunks that have changes within the given range.
func (f *FileDiff) HunksInRange(start, end int) []*Hunk {
	var result []*Hunk

	for _, hunk := range f.Hunks {
		if hunk.ContainsRange(start, end) {
			result = append(result, hunk)
		}
	}

	return result
}

// Format returns the file diff in unified diff format.
func (f *FileDiff) Format() string {
	var sb strings.Builder

	// File header.
	fmt.Fprintf(&sb, "--- a/%s\n", f.OldName)
	fmt.Fprintf(&sb, "+++ b/%s\n", f.NewName)

	// Hunks.
	for _, hunk := range f.Hunks {
		sb.WriteString(hunk.Header())
		sb.WriteByte('\n')

		for _, line := range hunk.Lines {
			sb.WriteString(line.String())
			sb.WriteByte('\n')
		}
	}

	return sb.String()
}
