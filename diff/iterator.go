package diff

import "iter"

// FilteredLines returns an iterator over lines matching a predicate.
func FilteredLines(
	lines iter.Seq[DiffLine], pred func(DiffLine) bool,
) iter.Seq[DiffLine] {

	return func(yield func(DiffLine) bool) {
		for line := range lines {
			if pred(line) {
				if !yield(line) {
					return
				}
			}
		}
	}
}

// MapLines transforms lines using a mapping function.
func MapLines[T any](
	lines iter.Seq[DiffLine], fn func(DiffLine) T,
) iter.Seq[T] {

	return func(yield func(T) bool) {
		for line := range lines {
			if !yield(fn(line)) {
				return
			}
		}
	}
}

// CollectLines collects all lines from an iterator into a slice.
func CollectLines(lines iter.Seq[DiffLine]) []DiffLine {
	var result []DiffLine

	for line := range lines {
		result = append(result, line)
	}

	return result
}

// CountLines counts the number of lines in an iterator.
func CountLines(lines iter.Seq[DiffLine]) int {
	count := 0

	for range lines {
		count++
	}

	return count
}

// TakeLines returns an iterator that yields at most n lines.
func TakeLines(lines iter.Seq[DiffLine], n int) iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		count := 0

		for line := range lines {
			if count >= n {
				return
			}

			if !yield(line) {
				return
			}

			count++
		}
	}
}

// SkipLines returns an iterator that skips the first n lines.
func SkipLines(lines iter.Seq[DiffLine], n int) iter.Seq[DiffLine] {
	return func(yield func(DiffLine) bool) {
		count := 0

		for line := range lines {
			count++
			if count <= n {
				continue
			}

			if !yield(line) {
				return
			}
		}
	}
}

// ChunkByOp groups consecutive lines by their operation type.
func ChunkByOp(lines iter.Seq[DiffLine]) iter.Seq[[]DiffLine] {
	return func(yield func([]DiffLine) bool) {
		var chunk []DiffLine
		var lastOp LineOp = -1

		for line := range lines {
			if lastOp != -1 && line.Op != lastOp {
				if len(chunk) > 0 {
					if !yield(chunk) {
						return
					}

					chunk = nil
				}
			}

			chunk = append(chunk, line)
			lastOp = line.Op
		}

		if len(chunk) > 0 {
			yield(chunk)
		}
	}
}

// ZipWithIndex pairs each line with its index.
func ZipWithIndex(lines iter.Seq[DiffLine]) iter.Seq2[int, DiffLine] {
	return func(yield func(int, DiffLine) bool) {
		idx := 0

		for line := range lines {
			if !yield(idx, line) {
				return
			}

			idx++
		}
	}
}

// LinesInRange returns an iterator over lines with NewLineNum in the range.
func LinesInRange(lines iter.Seq[DiffLine], start, end int) iter.Seq[DiffLine] {
	return FilteredLines(lines, func(line DiffLine) bool {
		// For deletions, check OldLineNum.
		if line.Op == OpDelete {
			return line.OldLineNum >= start && line.OldLineNum <= end
		}

		// For additions and context, check NewLineNum.
		return line.NewLineNum >= start && line.NewLineNum <= end
	})
}

// SelectedLines returns lines matching any of the selections.
func SelectedLines(
	lines iter.Seq[DiffLine], sel *FileSelection,
) iter.Seq[DiffLine] {

	return FilteredLines(lines, func(line DiffLine) bool {
		// For deletions, check OldLineNum.
		if line.Op == OpDelete {
			return sel.Contains(line.OldLineNum)
		}

		// For additions and context, check NewLineNum.
		return sel.Contains(line.NewLineNum)
	})
}

// ForEach applies a function to each line.
func ForEach(lines iter.Seq[DiffLine], fn func(DiffLine)) {
	for line := range lines {
		fn(line)
	}
}

// Any returns true if any line matches the predicate.
func Any(lines iter.Seq[DiffLine], pred func(DiffLine) bool) bool {
	for line := range lines {
		if pred(line) {
			return true
		}
	}

	return false
}

// All returns true if all lines match the predicate.
func All(lines iter.Seq[DiffLine], pred func(DiffLine) bool) bool {
	for line := range lines {
		if !pred(line) {
			return false
		}
	}

	return true
}

// FindFirst returns the first line matching the predicate.
func FindFirst(
	lines iter.Seq[DiffLine], pred func(DiffLine) bool,
) (DiffLine, bool) {

	for line := range lines {
		if pred(line) {
			return line, true
		}
	}

	return DiffLine{}, false
}
