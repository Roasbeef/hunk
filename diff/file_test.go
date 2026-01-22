package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func makeTestFileDiff() *diff.FileDiff {
	return &diff.FileDiff{
		OldName: "main.go",
		NewName: "main.go",
		Hunks: []*diff.Hunk{
			{
				OldStart: 1,
				NewStart: 1,
				Lines: []diff.DiffLine{
					{
						Op: diff.OpContext, Content: "package main",
						OldLineNum: 1, NewLineNum: 1,
					},
					{
						Op: diff.OpAdd, Content: "// Comment",
						OldLineNum: 0, NewLineNum: 2,
					},
				},
			},
			{
				OldStart: 10,
				NewStart: 11,
				Lines: []diff.DiffLine{
					{
						Op: diff.OpDelete, Content: "old line",
						OldLineNum: 10, NewLineNum: 0,
					},
					{
						Op: diff.OpAdd, Content: "new line",
						OldLineNum: 0, NewLineNum: 11,
					},
				},
			},
		},
	}
}

func TestFileDiff_Path(t *testing.T) {
	tests := []struct {
		name     string
		file     *diff.FileDiff
		wantPath string
	}{
		{
			name:     "normal modification",
			file:     &diff.FileDiff{OldName: "a.go", NewName: "a.go"},
			wantPath: "a.go",
		},
		{
			name: "deleted file",
			file: &diff.FileDiff{
				OldName: "old.go", NewName: "old.go", IsDeleted: true,
			},
			wantPath: "old.go",
		},
		{
			name: "new file",
			file: &diff.FileDiff{
				OldName: "", NewName: "new.go", IsNew: true,
			},
			wantPath: "new.go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantPath, tc.file.Path())
		})
	}
}

func TestFileDiff_AllHunks(t *testing.T) {
	file := makeTestFileDiff()

	var hunks []*diff.Hunk
	var indices []int
	for i, h := range file.AllHunks() {
		indices = append(indices, i)
		hunks = append(hunks, h)
	}

	require.Len(t, hunks, 2)
	require.Equal(t, []int{0, 1}, indices)
	require.Equal(t, 1, hunks[0].OldStart)
	require.Equal(t, 10, hunks[1].OldStart)
}

func TestFileDiff_AllLines(t *testing.T) {
	file := makeTestFileDiff()

	var hunkIndices []int
	var lines []diff.DiffLine
	for i, line := range file.AllLines() {
		hunkIndices = append(hunkIndices, i)
		lines = append(lines, line)
	}

	require.Len(t, lines, 4) // 2 lines in each hunk
	require.Equal(t, []int{0, 0, 1, 1}, hunkIndices)
}

func TestFileDiff_AllChanges(t *testing.T) {
	file := makeTestFileDiff()

	var changes []diff.DiffLine
	for _, line := range file.AllChanges() {
		changes = append(changes, line)
	}

	require.Len(t, changes, 3) // 1 add in first hunk, 1 delete + 1 add in second

	for _, c := range changes {
		require.True(t, c.IsChange())
	}
}

func TestFileDiff_Stats(t *testing.T) {
	file := makeTestFileDiff()

	added, deleted := file.Stats()
	require.Equal(t, 2, added)   // 2 adds total
	require.Equal(t, 1, deleted) // 1 delete total
}

func TestFileDiff_HunkContainingLine(t *testing.T) {
	file := makeTestFileDiff()

	// Line 2 is in first hunk (added line).
	hunk := file.HunkContainingLine(2)
	require.NotNil(t, hunk)
	require.Equal(t, 1, hunk.OldStart)

	// Line 11 is in second hunk (added line).
	hunk = file.HunkContainingLine(11)
	require.NotNil(t, hunk)
	require.Equal(t, 10, hunk.OldStart)

	// Line 100 doesn't exist.
	hunk = file.HunkContainingLine(100)
	require.Nil(t, hunk)
}

func TestFileDiff_HunksInRange(t *testing.T) {
	file := makeTestFileDiff()

	// Range covering first hunk.
	hunks := file.HunksInRange(1, 5)
	require.Len(t, hunks, 1)
	require.Equal(t, 1, hunks[0].OldStart)

	// Range covering second hunk.
	hunks = file.HunksInRange(10, 15)
	require.Len(t, hunks, 1)
	require.Equal(t, 10, hunks[0].OldStart)

	// Range covering both hunks.
	hunks = file.HunksInRange(1, 20)
	require.Len(t, hunks, 2)

	// Range covering nothing.
	hunks = file.HunksInRange(50, 60)
	require.Len(t, hunks, 0)
}

func TestFileDiff_Format(t *testing.T) {
	file := &diff.FileDiff{
		OldName: "test.go",
		NewName: "test.go",
		Hunks: []*diff.Hunk{
			{
				OldStart: 1,
				OldLines: 2,
				NewStart: 1,
				NewLines: 3,
				Lines: []diff.DiffLine{
					{Op: diff.OpContext, Content: "line1"},
					{Op: diff.OpAdd, Content: "new"},
					{Op: diff.OpContext, Content: "line2"},
				},
			},
		},
	}

	formatted := file.Format()

	require.Contains(t, formatted, "--- a/test.go")
	require.Contains(t, formatted, "+++ b/test.go")
	require.Contains(t, formatted, "@@ -1,2 +1,3 @@")
	require.Contains(t, formatted, " line1")
	require.Contains(t, formatted, "+new")
	require.Contains(t, formatted, " line2")
}
