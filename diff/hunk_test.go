package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func makeTestHunk() *diff.Hunk {
	return &diff.Hunk{
		OldStart: 10,
		OldLines: 5,
		NewStart: 10,
		NewLines: 7,
		Section:  "func example()",
		Lines: []diff.DiffLine{
			{Op: diff.OpContext, Content: "context1", OldLineNum: 10, NewLineNum: 10},
			{Op: diff.OpDelete, Content: "deleted", OldLineNum: 11, NewLineNum: 0},
			{Op: diff.OpAdd, Content: "added1", OldLineNum: 0, NewLineNum: 11},
			{Op: diff.OpAdd, Content: "added2", OldLineNum: 0, NewLineNum: 12},
			{Op: diff.OpContext, Content: "context2", OldLineNum: 12, NewLineNum: 13},
		},
	}
}

func TestHunk_Header(t *testing.T) {
	hunk := makeTestHunk()
	want := "@@ -10,5 +10,7 @@ func example()"
	require.Equal(t, want, hunk.Header())

	// Without section.
	hunk.Section = ""
	want = "@@ -10,5 +10,7 @@"
	require.Equal(t, want, hunk.Header())
}

func TestHunk_All(t *testing.T) {
	hunk := makeTestHunk()

	var lines []diff.DiffLine
	for line := range hunk.All() {
		lines = append(lines, line)
	}

	require.Len(t, lines, 5)
	require.Equal(t, "context1", lines[0].Content)
	require.Equal(t, "context2", lines[4].Content)
}

func TestHunk_All_EarlyTermination(t *testing.T) {
	hunk := makeTestHunk()

	count := 0
	for range hunk.All() {
		count++
		if count == 2 {
			break
		}
	}

	require.Equal(t, 2, count)
}

func TestHunk_Changes(t *testing.T) {
	hunk := makeTestHunk()

	var changes []diff.DiffLine
	for line := range hunk.Changes() {
		changes = append(changes, line)
	}

	require.Len(t, changes, 3) // 1 delete + 2 adds
	require.Equal(t, diff.OpDelete, changes[0].Op)
	require.Equal(t, diff.OpAdd, changes[1].Op)
	require.Equal(t, diff.OpAdd, changes[2].Op)
}

func TestHunk_Additions(t *testing.T) {
	hunk := makeTestHunk()

	var adds []diff.DiffLine
	for line := range hunk.Additions() {
		adds = append(adds, line)
	}

	require.Len(t, adds, 2)
	require.Equal(t, "added1", adds[0].Content)
	require.Equal(t, "added2", adds[1].Content)
}

func TestHunk_Deletions(t *testing.T) {
	hunk := makeTestHunk()

	var dels []diff.DiffLine
	for line := range hunk.Deletions() {
		dels = append(dels, line)
	}

	require.Len(t, dels, 1)
	require.Equal(t, "deleted", dels[0].Content)
}

func TestHunk_Stats(t *testing.T) {
	hunk := makeTestHunk()

	added, deleted := hunk.Stats()
	require.Equal(t, 2, added)
	require.Equal(t, 1, deleted)
}

func TestHunk_CanSplit(t *testing.T) {
	tests := []struct {
		name  string
		lines []diff.DiffLine
		want  bool
	}{
		{
			name: "cannot split - single change group",
			lines: []diff.DiffLine{
				{Op: diff.OpContext},
				{Op: diff.OpAdd},
				{Op: diff.OpAdd},
				{Op: diff.OpContext},
			},
			want: false,
		},
		{
			name: "can split - two change groups",
			lines: []diff.DiffLine{
				{Op: diff.OpContext},
				{Op: diff.OpAdd},
				{Op: diff.OpContext},
				{Op: diff.OpContext},
				{Op: diff.OpDelete},
				{Op: diff.OpContext},
			},
			want: true,
		},
		{
			name: "cannot split - only context",
			lines: []diff.DiffLine{
				{Op: diff.OpContext},
				{Op: diff.OpContext},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hunk := &diff.Hunk{Lines: tc.lines}
			require.Equal(t, tc.want, hunk.CanSplit())
		})
	}
}

func TestHunk_ContainsLine(t *testing.T) {
	hunk := makeTestHunk()

	// Added lines use NewLineNum.
	require.True(t, hunk.ContainsLine(11))  // added1
	require.True(t, hunk.ContainsLine(12))  // added2
	require.True(t, hunk.ContainsLine(11))  // deleted uses OldLineNum

	// Context lines are not changes.
	require.False(t, hunk.ContainsLine(10)) // context
	require.False(t, hunk.ContainsLine(99)) // nonexistent
}

func TestHunk_ContainsRange(t *testing.T) {
	hunk := makeTestHunk()

	require.True(t, hunk.ContainsRange(10, 15))
	require.True(t, hunk.ContainsRange(11, 11))
	require.False(t, hunk.ContainsRange(1, 5))
	require.False(t, hunk.ContainsRange(100, 200))
}

func TestHunk_RecalculateLineCounts(t *testing.T) {
	hunk := &diff.Hunk{
		Lines: []diff.DiffLine{
			{Op: diff.OpContext},
			{Op: diff.OpDelete},
			{Op: diff.OpAdd},
			{Op: diff.OpAdd},
			{Op: diff.OpContext},
		},
	}

	hunk.RecalculateLineCounts()

	// Context: 2 (both old and new).
	// Delete: 1 old.
	// Add: 2 new.
	require.Equal(t, 3, hunk.OldLines) // 2 context + 1 delete
	require.Equal(t, 4, hunk.NewLines) // 2 context + 2 add
}
