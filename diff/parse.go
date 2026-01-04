package diff

import (
	"bytes"
	"fmt"
	"iter"
	"strings"

	godiff "github.com/sourcegraph/go-diff/diff"
)

// ParsedDiff wraps a parsed multi-file diff.
type ParsedDiff struct {
	files []*FileDiff
}

// Parse parses a unified diff string into a structured representation.
func Parse(diffText string) (*ParsedDiff, error) {
	if strings.TrimSpace(diffText) == "" {
		return &ParsedDiff{}, nil
	}

	files, err := godiff.ParseMultiFileDiff([]byte(diffText))
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}

	parsed := &ParsedDiff{
		files: make([]*FileDiff, 0, len(files)),
	}

	for _, f := range files {
		fd := convertFileDiff(f)
		parsed.files = append(parsed.files, fd)
	}

	return parsed, nil
}

// Files returns an iterator over all file diffs.
func (d *ParsedDiff) Files() iter.Seq[*FileDiff] {
	return func(yield func(*FileDiff) bool) {
		for _, f := range d.files {
			if !yield(f) {
				return
			}
		}
	}
}

// FilesWithIndex returns an iterator with indices.
func (d *ParsedDiff) FilesWithIndex() iter.Seq2[int, *FileDiff] {
	return func(yield func(int, *FileDiff) bool) {
		for i, f := range d.files {
			if !yield(i, f) {
				return
			}
		}
	}
}

// FileCount returns the number of files in the diff.
func (d *ParsedDiff) FileCount() int {
	return len(d.files)
}

// FileByPath finds a file diff by path.
func (d *ParsedDiff) FileByPath(path string) *FileDiff {
	for _, f := range d.files {
		if f.Path() == path || f.OldName == path || f.NewName == path {
			return f
		}
	}

	return nil
}

// AllFiles returns a slice of all file diffs.
func (d *ParsedDiff) AllFiles() []*FileDiff {
	return d.files
}

// Stats returns total addition and deletion counts across all files.
func (d *ParsedDiff) Stats() (added, deleted int) {
	for _, f := range d.files {
		a, del := f.Stats()
		added += a
		deleted += del
	}

	return added, deleted
}

// LineWithContext provides full context for a diff line.
type LineWithContext struct {
	// GlobalIndex is the index of this line across all files.
	GlobalIndex int

	// File is the file containing this line.
	File *FileDiff

	// HunkIndex is the index of the hunk within the file.
	HunkIndex int

	// LineIndex is the index of the line within the hunk.
	LineIndex int

	// Line is the actual diff line.
	Line DiffLine
}

// LinesWithContext returns an iterator over all lines with full context.
func (d *ParsedDiff) LinesWithContext() iter.Seq[LineWithContext] {
	return func(yield func(LineWithContext) bool) {
		globalIdx := 0

		for _, f := range d.files {
			for hunkIdx, hunk := range f.Hunks {
				for lineIdx, line := range hunk.Lines {
					ctx := LineWithContext{
						GlobalIndex: globalIdx,
						File:        f,
						HunkIndex:   hunkIdx,
						LineIndex:   lineIdx,
						Line:        line,
					}
					if !yield(ctx) {
						return
					}
					globalIdx++
				}
			}
		}
	}
}

// convertFileDiff converts from go-diff types to our types.
func convertFileDiff(f *godiff.FileDiff) *FileDiff {
	fd := &FileDiff{
		OldName:   stripPrefix(f.OrigName),
		NewName:   stripPrefix(f.NewName),
		IsNew:     f.OrigName == "/dev/null",
		IsDeleted: f.NewName == "/dev/null",
	}

	// Check for renames.
	if fd.OldName != fd.NewName && !fd.IsNew && !fd.IsDeleted {
		fd.IsRenamed = true
	}

	// Check for binary.
	for _, ex := range f.Extended {
		if strings.Contains(ex, "Binary files") {
			fd.IsBinary = true

			break
		}
	}

	for _, h := range f.Hunks {
		hunk := convertHunk(h)
		fd.Hunks = append(fd.Hunks, hunk)
	}

	return fd
}

// convertHunk converts a go-diff Hunk to our Hunk type with line numbers.
func convertHunk(h *godiff.Hunk) *Hunk {
	hunk := &Hunk{
		OldStart: int(h.OrigStartLine),
		OldLines: int(h.OrigLines),
		NewStart: int(h.NewStartLine),
		NewLines: int(h.NewLines),
		Section:  h.Section,
	}

	// Parse the body to extract individual lines with numbers.
	oldLine := hunk.OldStart
	newLine := hunk.NewStart

	lines := bytes.Split(h.Body, []byte("\n"))
	for _, lineBytes := range lines {
		if len(lineBytes) == 0 {
			continue
		}

		prefix := lineBytes[0]
		content := string(lineBytes[1:])

		var dl DiffLine

		switch prefix {
		case ' ':
			dl = DiffLine{
				Op:         OpContext,
				Content:    content,
				OldLineNum: oldLine,
				NewLineNum: newLine,
			}
			oldLine++
			newLine++

		case '+':
			dl = DiffLine{
				Op:         OpAdd,
				Content:    content,
				OldLineNum: 0,
				NewLineNum: newLine,
			}
			newLine++

		case '-':
			dl = DiffLine{
				Op:         OpDelete,
				Content:    content,
				OldLineNum: oldLine,
				NewLineNum: 0,
			}
			oldLine++

		case '\\':
			// Handle "\ No newline at end of file" - skip.
			continue

		default:
			// Unknown prefix, skip.
			continue
		}

		hunk.Lines = append(hunk.Lines, dl)
	}

	return hunk
}

// stripPrefix removes the "a/" or "b/" prefix from git diff paths.
func stripPrefix(path string) string {
	if strings.HasPrefix(path, "a/") || strings.HasPrefix(path, "b/") {
		return path[2:]
	}

	return path
}
