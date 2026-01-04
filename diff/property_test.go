package diff_test

import (
	"fmt"
	"testing"

	"github.com/roasbeef/hunk/diff"
	"pgregory.net/rapid"
)

// TestLineRangeContainsProperty verifies Contains behavior with rapid.
func TestLineRangeContainsProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		start := rapid.IntRange(1, 1000).Draw(t, "start")
		length := rapid.IntRange(0, 100).Draw(t, "length")
		end := start + length

		r := diff.LineRange{Start: start, End: end}

		// Property: All lines in range should be contained.
		for i := start; i <= end; i++ {
			if !r.Contains(i) {
				t.Fatalf("range [%d-%d] should contain %d", start, end, i)
			}
		}

		// Property: Lines before start should not be contained.
		if start > 1 && r.Contains(start-1) {
			t.Fatalf("range [%d-%d] should not contain %d", start, end, start-1)
		}

		// Property: Lines after end should not be contained.
		if r.Contains(end + 1) {
			t.Fatalf("range [%d-%d] should not contain %d", start, end, end+1)
		}
	})
}

// TestLineRangeStringRoundtrip verifies String produces valid range syntax.
func TestLineRangeStringRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		start := rapid.IntRange(1, 10000).Draw(t, "start")
		length := rapid.IntRange(0, 1000).Draw(t, "length")
		end := start + length

		r := diff.LineRange{Start: start, End: end}
		str := r.String()

		// Parse it back.
		sel, err := diff.ParseFileSelection("file.go:" + str)
		if err != nil {
			t.Fatalf("failed to parse range string %q: %v", str, err)
		}

		// Property: Should have exactly one range.
		if len(sel.Ranges) != 1 {
			t.Fatalf("expected 1 range, got %d", len(sel.Ranges))
		}

		// Property: Parsed range should match original.
		parsed := sel.Ranges[0]
		if parsed.Start != start || parsed.End != end {
			t.Fatalf("range mismatch: want [%d-%d], got [%d-%d]",
				start, end, parsed.Start, parsed.End)
		}
	})
}

// TestFileSelectionMergeProperty verifies merge preserves coverage.
func TestFileSelectionMergeProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numRanges := rapid.IntRange(1, 10).Draw(t, "numRanges")
		var ranges []diff.LineRange

		for i := 0; i < numRanges; i++ {
			start := rapid.IntRange(1, 100).Draw(t, fmt.Sprintf("start%d", i))
			length := rapid.IntRange(0, 20).Draw(t, fmt.Sprintf("length%d", i))
			ranges = append(ranges, diff.LineRange{Start: start, End: start + length})
		}

		sel := &diff.FileSelection{Path: "test.go", Ranges: ranges}

		// Collect all lines covered before merge.
		linesBefore := make(map[int]bool)
		for _, r := range sel.Ranges {
			for i := r.Start; i <= r.End; i++ {
				linesBefore[i] = true
			}
		}

		// Merge.
		sel.Merge()

		// Collect all lines covered after merge.
		linesAfter := make(map[int]bool)
		for _, r := range sel.Ranges {
			for i := r.Start; i <= r.End; i++ {
				linesAfter[i] = true
			}
		}

		// Property: Same lines should be covered.
		for line := range linesBefore {
			if !linesAfter[line] {
				t.Fatalf("line %d lost after merge", line)
			}
		}
		for line := range linesAfter {
			if !linesBefore[line] {
				t.Fatalf("line %d added after merge", line)
			}
		}

		// Property: Ranges should be sorted by start.
		for i := 1; i < len(sel.Ranges); i++ {
			if sel.Ranges[i].Start <= sel.Ranges[i-1].End {
				t.Fatalf("ranges not properly sorted/merged: %v", sel.Ranges)
			}
		}
	})
}

// TestParseFileSelectionProperty tests parsing with generated inputs.
func TestParseFileSelectionProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate valid path.
		pathParts := rapid.IntRange(1, 3).Draw(t, "pathParts")
		path := ""
		for i := 0; i < pathParts; i++ {
			if i > 0 {
				path += "/"
			}
			path += rapid.StringMatching(`[a-z][a-z0-9_]*`).Draw(t, fmt.Sprintf("part%d", i))
		}
		path += ".go"

		// Generate valid ranges.
		numRanges := rapid.IntRange(1, 5).Draw(t, "numRanges")
		rangeStrs := make([]string, numRanges)
		for i := 0; i < numRanges; i++ {
			start := rapid.IntRange(1, 1000).Draw(t, fmt.Sprintf("start%d", i))
			isSingle := rapid.Bool().Draw(t, fmt.Sprintf("single%d", i))
			if isSingle {
				rangeStrs[i] = fmt.Sprintf("%d", start)
			} else {
				length := rapid.IntRange(1, 100).Draw(t, fmt.Sprintf("len%d", i))
				rangeStrs[i] = fmt.Sprintf("%d-%d", start, start+length)
			}
		}

		// Build selection string.
		rangeSpec := rangeStrs[0]
		for i := 1; i < len(rangeStrs); i++ {
			rangeSpec += "," + rangeStrs[i]
		}
		input := path + ":" + rangeSpec

		// Parse it.
		sel, err := diff.ParseFileSelection(input)
		if err != nil {
			t.Fatalf("failed to parse %q: %v", input, err)
		}

		// Property: Path should match.
		if sel.Path != path {
			t.Fatalf("path mismatch: want %q, got %q", path, sel.Path)
		}

		// Property: Number of ranges should match.
		if len(sel.Ranges) != numRanges {
			t.Fatalf("range count mismatch: want %d, got %d", numRanges, len(sel.Ranges))
		}
	})
}

// TestDiffLineOpSymmetry verifies Op methods are consistent.
func TestDiffLineOpSymmetry(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		op := diff.LineOp(rapid.IntRange(0, 2).Draw(t, "op"))

		// Property: Prefix should be consistent with Op.
		prefix := op.Prefix()
		switch op {
		case diff.OpContext:
			if prefix != ' ' {
				t.Fatalf("context should have space prefix, got %c", prefix)
			}
		case diff.OpAdd:
			if prefix != '+' {
				t.Fatalf("add should have + prefix, got %c", prefix)
			}
		case diff.OpDelete:
			if prefix != '-' {
				t.Fatalf("delete should have - prefix, got %c", prefix)
			}
		}

		// Property: String should be non-empty.
		str := op.String()
		if str == "" {
			t.Fatal("op string should not be empty")
		}
		if str == "unknown" && op <= 2 {
			t.Fatal("valid op should not be unknown")
		}
	})
}

// TestDiffLineIsChange verifies IsChange is consistent with Op.
func TestDiffLineIsChange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		op := diff.LineOp(rapid.IntRange(0, 2).Draw(t, "op"))
		line := diff.DiffLine{Op: op}

		isChange := line.IsChange()

		// Property: Only add and delete are changes.
		expectedChange := op == diff.OpAdd || op == diff.OpDelete
		if isChange != expectedChange {
			t.Fatalf("IsChange for op %v: want %v, got %v", op, expectedChange, isChange)
		}
	})
}

// TestHunkStatsConsistency verifies Stats matches line counts.
func TestHunkStatsConsistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numLines := rapid.IntRange(1, 20).Draw(t, "numLines")
		var lines []diff.DiffLine

		expectedAdds := 0
		expectedDels := 0

		for i := 0; i < numLines; i++ {
			op := diff.LineOp(rapid.IntRange(0, 2).Draw(t, fmt.Sprintf("op%d", i)))
			lines = append(lines, diff.DiffLine{Op: op})

			switch op {
			case diff.OpAdd:
				expectedAdds++
			case diff.OpDelete:
				expectedDels++
			}
		}

		hunk := &diff.Hunk{Lines: lines}
		added, deleted := hunk.Stats()

		if added != expectedAdds {
			t.Fatalf("added mismatch: want %d, got %d", expectedAdds, added)
		}
		if deleted != expectedDels {
			t.Fatalf("deleted mismatch: want %d, got %d", expectedDels, deleted)
		}
	})
}

// FuzzParseFileSelection uses MakeFuzz for property-based fuzzing.
func FuzzParseFileSelection(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		// Generate a valid file selection and verify roundtrip.
		path := rapid.StringMatching(`[a-z][a-z0-9/]*\.go`).Draw(t, "path")
		start := rapid.IntRange(1, 10000).Draw(t, "start")
		length := rapid.IntRange(0, 1000).Draw(t, "length")

		input := fmt.Sprintf("%s:%d-%d", path, start, start+length)
		sel, err := diff.ParseFileSelection(input)
		if err != nil {
			t.Fatalf("valid input %q failed: %v", input, err)
		}

		if sel.Path != path {
			t.Fatalf("path mismatch")
		}
		if len(sel.Ranges) != 1 {
			t.Fatalf("expected 1 range")
		}
		if sel.Ranges[0].Start != start {
			t.Fatalf("start mismatch")
		}
	}))
}

// FuzzLineRangeContains uses MakeFuzz for range containment fuzzing.
func FuzzLineRangeContains(f *testing.F) {
	f.Fuzz(rapid.MakeFuzz(func(t *rapid.T) {
		start := rapid.IntRange(1, 10000).Draw(t, "start")
		end := rapid.IntRange(start, start+1000).Draw(t, "end")
		testLine := rapid.IntRange(1, 20000).Draw(t, "testLine")

		r := diff.LineRange{Start: start, End: end}
		contains := r.Contains(testLine)

		expected := testLine >= start && testLine <= end
		if contains != expected {
			t.Fatalf("Contains(%d) for [%d-%d]: got %v, want %v",
				testLine, start, end, contains, expected)
		}
	}))
}
