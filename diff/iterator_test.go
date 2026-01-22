package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func makeTestLines() []diff.DiffLine {
	return []diff.DiffLine{
		{Op: diff.OpContext, Content: "ctx1", OldLineNum: 1, NewLineNum: 1},
		{Op: diff.OpAdd, Content: "add1", OldLineNum: 0, NewLineNum: 2},
		{Op: diff.OpAdd, Content: "add2", OldLineNum: 0, NewLineNum: 3},
		{Op: diff.OpDelete, Content: "del1", OldLineNum: 2, NewLineNum: 0},
		{Op: diff.OpContext, Content: "ctx2", OldLineNum: 3, NewLineNum: 4},
	}
}

func linesIter(lines []diff.DiffLine) func(yield func(diff.DiffLine) bool) {
	return func(yield func(diff.DiffLine) bool) {
		for _, line := range lines {
			if !yield(line) {
				return
			}
		}
	}
}

func TestFilteredLines(t *testing.T) {
	lines := makeTestLines()

	// Filter for adds only.
	var adds []diff.DiffLine
	for line := range diff.FilteredLines(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Op == diff.OpAdd
	}) {
		adds = append(adds, line)
	}

	require.Len(t, adds, 2)
	require.Equal(t, "add1", adds[0].Content)
	require.Equal(t, "add2", adds[1].Content)
}

func TestMapLines(t *testing.T) {
	lines := makeTestLines()

	// Map to content strings.
	var contents []string
	for content := range diff.MapLines(linesIter(lines), func(l diff.DiffLine) string {
		return l.Content
	}) {
		contents = append(contents, content)
	}

	require.Equal(t, []string{"ctx1", "add1", "add2", "del1", "ctx2"}, contents)
}

func TestCollectLines(t *testing.T) {
	lines := makeTestLines()

	collected := diff.CollectLines(linesIter(lines))
	require.Len(t, collected, 5)
	require.Equal(t, lines, collected)
}

func TestCountLines(t *testing.T) {
	lines := makeTestLines()

	count := diff.CountLines(linesIter(lines))
	require.Equal(t, 5, count)

	// Empty iterator.
	count = diff.CountLines(linesIter(nil))
	require.Equal(t, 0, count)
}

func TestTakeLines(t *testing.T) {
	lines := makeTestLines()

	var taken []diff.DiffLine
	for line := range diff.TakeLines(linesIter(lines), 3) {
		taken = append(taken, line)
	}

	require.Len(t, taken, 3)
	require.Equal(t, "ctx1", taken[0].Content)
	require.Equal(t, "add1", taken[1].Content)
	require.Equal(t, "add2", taken[2].Content)

	// Take more than available.
	taken = nil
	for line := range diff.TakeLines(linesIter(lines), 100) {
		taken = append(taken, line)
	}
	require.Len(t, taken, 5)
}

func TestSkipLines(t *testing.T) {
	lines := makeTestLines()

	var skipped []diff.DiffLine
	for line := range diff.SkipLines(linesIter(lines), 2) {
		skipped = append(skipped, line)
	}

	require.Len(t, skipped, 3)
	require.Equal(t, "add2", skipped[0].Content)
	require.Equal(t, "del1", skipped[1].Content)
	require.Equal(t, "ctx2", skipped[2].Content)

	// Skip all.
	skipped = nil
	for line := range diff.SkipLines(linesIter(lines), 100) {
		skipped = append(skipped, line)
	}
	require.Empty(t, skipped)
}

func TestChunkByOp(t *testing.T) {
	lines := []diff.DiffLine{
		{Op: diff.OpContext, Content: "c1"},
		{Op: diff.OpContext, Content: "c2"},
		{Op: diff.OpAdd, Content: "a1"},
		{Op: diff.OpAdd, Content: "a2"},
		{Op: diff.OpDelete, Content: "d1"},
		{Op: diff.OpContext, Content: "c3"},
	}

	var chunks [][]diff.DiffLine
	for chunk := range diff.ChunkByOp(linesIter(lines)) {
		chunks = append(chunks, chunk)
	}

	require.Len(t, chunks, 4)
	require.Len(t, chunks[0], 2) // 2 context lines
	require.Len(t, chunks[1], 2) // 2 adds
	require.Len(t, chunks[2], 1) // 1 delete
	require.Len(t, chunks[3], 1) // 1 context
}

func TestZipWithIndex(t *testing.T) {
	lines := makeTestLines()

	var indices []int
	var collected []diff.DiffLine
	for i, line := range diff.ZipWithIndex(linesIter(lines)) {
		indices = append(indices, i)
		collected = append(collected, line)
	}

	require.Equal(t, []int{0, 1, 2, 3, 4}, indices)
	require.Len(t, collected, 5)
}

func TestLinesInRange(t *testing.T) {
	lines := makeTestLines()

	var inRange []diff.DiffLine
	for line := range diff.LinesInRange(linesIter(lines), 1, 2) {
		inRange = append(inRange, line)
	}

	// Should include:
	// - ctx1 (old:1, new:1) - new in range
	// - add1 (old:0, new:2) - new in range
	// - del1 (old:2, new:0) - old in range
	require.Len(t, inRange, 3)
}

func TestSelectedLines(t *testing.T) {
	lines := makeTestLines()

	sel := &diff.FileSelection{
		Path:   "test.go",
		Ranges: []diff.LineRange{{Start: 2, End: 3}},
	}

	var selected []diff.DiffLine
	for line := range diff.SelectedLines(linesIter(lines), sel) {
		selected = append(selected, line)
	}

	// Should include lines where new line (for add/ctx) or old line (for del) is in [2,3].
	require.Len(t, selected, 3) // add1 (new:2), add2 (new:3), del1 (old:2)
}

func TestForEach(t *testing.T) {
	lines := makeTestLines()

	var contents []string
	diff.ForEach(linesIter(lines), func(l diff.DiffLine) {
		contents = append(contents, l.Content)
	})

	require.Equal(t, []string{"ctx1", "add1", "add2", "del1", "ctx2"}, contents)
}

func TestAny(t *testing.T) {
	lines := makeTestLines()

	// Has adds.
	hasAdd := diff.Any(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Op == diff.OpAdd
	})
	require.True(t, hasAdd)

	// No unknown op.
	hasUnknown := diff.Any(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Op == diff.LineOp(99)
	})
	require.False(t, hasUnknown)
}

func TestAll(t *testing.T) {
	// All context lines.
	ctxLines := []diff.DiffLine{
		{Op: diff.OpContext},
		{Op: diff.OpContext},
	}

	allContext := diff.All(linesIter(ctxLines), func(l diff.DiffLine) bool {
		return l.Op == diff.OpContext
	})
	require.True(t, allContext)

	// Not all context.
	lines := makeTestLines()
	allContext = diff.All(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Op == diff.OpContext
	})
	require.False(t, allContext)
}

func TestFindFirst(t *testing.T) {
	lines := makeTestLines()

	// Find first add.
	line, found := diff.FindFirst(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Op == diff.OpAdd
	})
	require.True(t, found)
	require.Equal(t, "add1", line.Content)

	// Find non-existent.
	_, found = diff.FindFirst(linesIter(lines), func(l diff.DiffLine) bool {
		return l.Content == "nonexistent"
	})
	require.False(t, found)
}
