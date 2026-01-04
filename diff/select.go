package diff

import (
	"fmt"
	"strconv"
	"strings"
)

// LineRange represents a range of lines to select.
type LineRange struct {
	Start int // Inclusive.
	End   int // Inclusive.
}

// Contains checks if a line number is within this range.
func (r LineRange) Contains(lineNum int) bool {
	return lineNum >= r.Start && lineNum <= r.End
}

// String returns the range as a string (e.g., "10-20" or "15").
func (r LineRange) String() string {
	if r.Start == r.End {
		return strconv.Itoa(r.Start)
	}

	return fmt.Sprintf("%d-%d", r.Start, r.End)
}

// FileSelection represents selected lines for a file.
type FileSelection struct {
	Path   string
	Ranges []LineRange
}

// ParseFileSelection parses "FILE:LINES" syntax.
// Examples:
//   - "main.go:10-20" - lines 10 through 20
//   - "main.go:10,15,20-25" - lines 10, 15, and 20-25
//   - "main.go:10" - just line 10
func ParseFileSelection(s string) (*FileSelection, error) {
	// Find the last colon to handle Windows paths like C:\path\file.go:10.
	lastColon := strings.LastIndex(s, ":")
	if lastColon == -1 {
		return nil, fmt.Errorf(
			"invalid selection syntax: expected FILE:LINES, got %q", s,
		)
	}

	path := s[:lastColon]
	rangeSpec := s[lastColon+1:]

	if path == "" {
		return nil, fmt.Errorf("empty file path in selection: %q", s)
	}

	if rangeSpec == "" {
		return nil, fmt.Errorf("empty line range in selection: %q", s)
	}

	var ranges []LineRange

	for _, part := range strings.Split(rangeSpec, ",") {
		r, err := parseRange(part)
		if err != nil {
			return nil, fmt.Errorf("invalid range %q in %q: %w", part, s, err)
		}

		ranges = append(ranges, r)
	}

	return &FileSelection{Path: path, Ranges: ranges}, nil
}

// parseRange parses a single range like "10", "10-20".
func parseRange(s string) (LineRange, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return LineRange{}, fmt.Errorf("empty range")
	}

	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		if len(parts) != 2 {
			return LineRange{}, fmt.Errorf("invalid range format")
		}

		start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			return LineRange{}, fmt.Errorf("invalid start line: %w", err)
		}

		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return LineRange{}, fmt.Errorf("invalid end line: %w", err)
		}

		if start > end {
			return LineRange{}, fmt.Errorf(
				"start line %d greater than end line %d", start, end,
			)
		}

		if start < 1 {
			return LineRange{}, fmt.Errorf("line numbers must be positive")
		}

		return LineRange{Start: start, End: end}, nil
	}

	line, err := strconv.Atoi(s)
	if err != nil {
		return LineRange{}, fmt.Errorf("invalid line number: %w", err)
	}

	if line < 1 {
		return LineRange{}, fmt.Errorf("line numbers must be positive")
	}

	return LineRange{Start: line, End: line}, nil
}

// Contains checks if a line number is within any of the ranges.
func (fs *FileSelection) Contains(lineNum int) bool {
	for _, r := range fs.Ranges {
		if r.Contains(lineNum) {
			return true
		}
	}

	return false
}

// String returns the selection as a string.
func (fs *FileSelection) String() string {
	var parts []string
	for _, r := range fs.Ranges {
		parts = append(parts, r.String())
	}

	return fs.Path + ":" + strings.Join(parts, ",")
}

// AllLines returns all individual line numbers covered by the ranges.
func (fs *FileSelection) AllLines() []int {
	var lines []int

	for _, r := range fs.Ranges {
		for i := r.Start; i <= r.End; i++ {
			lines = append(lines, i)
		}
	}

	return lines
}

// Merge merges overlapping and adjacent ranges.
func (fs *FileSelection) Merge() {
	if len(fs.Ranges) <= 1 {
		return
	}

	// Sort by start line.
	for i := 0; i < len(fs.Ranges); i++ {
		for j := i + 1; j < len(fs.Ranges); j++ {
			if fs.Ranges[j].Start < fs.Ranges[i].Start {
				fs.Ranges[i], fs.Ranges[j] = fs.Ranges[j], fs.Ranges[i]
			}
		}
	}

	// Merge overlapping.
	merged := []LineRange{fs.Ranges[0]}

	for i := 1; i < len(fs.Ranges); i++ {
		last := &merged[len(merged)-1]
		curr := fs.Ranges[i]

		if curr.Start <= last.End+1 {
			// Overlapping or adjacent, merge.
			if curr.End > last.End {
				last.End = curr.End
			}
		} else {
			merged = append(merged, curr)
		}
	}

	fs.Ranges = merged
}

// ParseSelections parses multiple FILE:LINES arguments.
func ParseSelections(args []string) ([]*FileSelection, error) {
	selections := make([]*FileSelection, 0, len(args))

	for _, arg := range args {
		sel, err := ParseFileSelection(arg)
		if err != nil {
			return nil, err
		}

		selections = append(selections, sel)
	}

	return selections, nil
}

// SelectionMap groups selections by file path for efficient lookup.
type SelectionMap map[string]*FileSelection

// NewSelectionMap creates a SelectionMap from a slice of selections.
func NewSelectionMap(selections []*FileSelection) SelectionMap {
	m := make(SelectionMap)

	for _, sel := range selections {
		if existing, ok := m[sel.Path]; ok {
			// Merge ranges for the same file.
			existing.Ranges = append(existing.Ranges, sel.Ranges...)
			existing.Merge()
		} else {
			m[sel.Path] = sel
		}
	}

	return m
}

// Get returns the selection for a file path, or nil if not found.
func (m SelectionMap) Get(path string) *FileSelection {
	return m[path]
}

// Contains checks if a specific line in a file is selected.
func (m SelectionMap) Contains(path string, lineNum int) bool {
	sel, ok := m[path]
	if !ok {
		return false
	}

	return sel.Contains(lineNum)
}
