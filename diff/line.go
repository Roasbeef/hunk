// Package diff provides types and functions for parsing and manipulating
// unified diffs.
package diff

import (
	"fmt"
	"strconv"
)

// LineOp represents the type of diff operation for a line.
type LineOp int

const (
	// OpContext indicates an unchanged line (context).
	OpContext LineOp = iota
	// OpAdd indicates an added line.
	OpAdd
	// OpDelete indicates a deleted line.
	OpDelete
)

// String returns the string representation of the operation.
func (op LineOp) String() string {
	switch op {
	case OpContext:
		return "context"
	case OpAdd:
		return "add"
	case OpDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// Prefix returns the diff prefix character for the operation.
func (op LineOp) Prefix() byte {
	switch op {
	case OpAdd:
		return '+'
	case OpDelete:
		return '-'
	default:
		return ' '
	}
}

// DiffLine represents a single line in a diff hunk.
//
//nolint:revive // Renaming to Line would be a breaking API change.
type DiffLine struct {
	// Op is the type of operation (context, add, delete).
	Op LineOp

	// Content is the line content without the prefix (+/-/space).
	Content string

	// OldLineNum is the line number in the original file.
	// Zero if this is an added line.
	OldLineNum int

	// NewLineNum is the line number in the new file.
	// Zero if this is a deleted line.
	NewLineNum int
}

// String returns the line in unified diff format.
func (l DiffLine) String() string {
	return string(l.Op.Prefix()) + l.Content
}

// LineRef returns a parseable reference for this line.
// Format: "OLD:NEW" where missing numbers are represented as "-".
func (l DiffLine) LineRef() string {
	old := "-"
	if l.OldLineNum > 0 {
		old = strconv.Itoa(l.OldLineNum)
	}

	newNum := "-"
	if l.NewLineNum > 0 {
		newNum = strconv.Itoa(l.NewLineNum)
	}

	return old + ":" + newNum
}

// IsChange returns true if this line represents a change (add or delete).
func (l DiffLine) IsChange() bool {
	return l.Op == OpAdd || l.Op == OpDelete
}

// EffectiveLineNum returns the relevant line number for selection purposes.
// For added lines, returns NewLineNum.
// For deleted and context lines, returns OldLineNum.
func (l DiffLine) EffectiveLineNum() int {
	if l.Op == OpAdd {
		return l.NewLineNum
	}

	return l.OldLineNum
}

// Format returns a human-readable format with line numbers.
func (l DiffLine) Format() string {
	var old string
	if l.OldLineNum > 0 {
		old = fmt.Sprintf("%4d", l.OldLineNum)
	} else {
		old = "    "
	}

	var newNum string
	if l.NewLineNum > 0 {
		newNum = fmt.Sprintf("%4d", l.NewLineNum)
	} else {
		newNum = "    "
	}

	return fmt.Sprintf("%s %s %c%s", old, newNum, l.Op.Prefix(), l.Content)
}
