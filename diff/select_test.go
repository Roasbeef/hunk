package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func TestParseFileSelection(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		wantPath  string
		wantLines []int
	}{
		{
			name:      "single line",
			input:     "main.go:10",
			wantPath:  "main.go",
			wantLines: []int{10},
		},
		{
			name:      "line range",
			input:     "main.go:10-20",
			wantPath:  "main.go",
			wantLines: []int{10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		},
		{
			name:      "multiple ranges",
			input:     "main.go:10-12,15,20-22",
			wantPath:  "main.go",
			wantLines: []int{10, 11, 12, 15, 20, 21, 22},
		},
		{
			name:      "path with directory",
			input:     "internal/pkg/file.go:5-10",
			wantPath:  "internal/pkg/file.go",
			wantLines: []int{5, 6, 7, 8, 9, 10},
		},
		{
			name:    "missing colon",
			input:   "main.go10-20",
			wantErr: true,
		},
		{
			name:    "empty path",
			input:   ":10-20",
			wantErr: true,
		},
		{
			name:    "empty range",
			input:   "main.go:",
			wantErr: true,
		},
		{
			name:    "invalid range format",
			input:   "main.go:abc",
			wantErr: true,
		},
		{
			name:    "start greater than end",
			input:   "main.go:20-10",
			wantErr: true,
		},
		{
			name:    "negative line",
			input:   "main.go:-5",
			wantErr: true,
		},
		{
			name:    "zero line",
			input:   "main.go:0",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sel, err := diff.ParseFileSelection(tc.input)
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantPath, sel.Path)
			require.Equal(t, tc.wantLines, sel.AllLines())
		})
	}
}

func TestFileSelectionContains(t *testing.T) {
	sel, err := diff.ParseFileSelection("main.go:10-20,30-40")
	require.NoError(t, err)

	// In range.
	require.True(t, sel.Contains(10))
	require.True(t, sel.Contains(15))
	require.True(t, sel.Contains(20))
	require.True(t, sel.Contains(30))
	require.True(t, sel.Contains(35))
	require.True(t, sel.Contains(40))

	// Out of range.
	require.False(t, sel.Contains(9))
	require.False(t, sel.Contains(21))
	require.False(t, sel.Contains(25))
	require.False(t, sel.Contains(29))
	require.False(t, sel.Contains(41))
}

func TestFileSelectionMerge(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRanges string
	}{
		{
			name:       "no overlap",
			input:      "f:10-20,30-40",
			wantRanges: "10-20,30-40",
		},
		{
			name:       "overlapping",
			input:      "f:10-20,15-25",
			wantRanges: "10-25",
		},
		{
			name:       "adjacent",
			input:      "f:10-20,21-30",
			wantRanges: "10-30",
		},
		{
			name:       "contained",
			input:      "f:10-40,15-25",
			wantRanges: "10-40",
		},
		{
			name:       "unsorted",
			input:      "f:30-40,10-20",
			wantRanges: "10-20,30-40",
		},
		{
			name:       "multiple merges",
			input:      "f:1-10,5-15,20-30,25-35",
			wantRanges: "1-15,20-35",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sel, err := diff.ParseFileSelection(tc.input)
			require.NoError(t, err)

			sel.Merge()

			// Reconstruct ranges string.
			var rangeStrs []string
			for _, r := range sel.Ranges {
				rangeStrs = append(rangeStrs, r.String())
			}

			result := ""
			for i, rs := range rangeStrs {
				if i > 0 {
					result += ","
				}
				result += rs
			}

			require.Equal(t, tc.wantRanges, result)
		})
	}
}

func TestSelectionMap(t *testing.T) {
	selections := []*diff.FileSelection{
		{Path: "main.go", Ranges: []diff.LineRange{{Start: 10, End: 20}}},
		{Path: "utils.go", Ranges: []diff.LineRange{{Start: 5, End: 10}}},
		{
			Path: "main.go",
			Ranges: []diff.LineRange{{Start: 30, End: 40}},
		}, // Same file.
	}

	m := diff.NewSelectionMap(selections)

	// Test Get.
	mainSel := m.Get("main.go")
	require.NotNil(t, mainSel)

	utilsSel := m.Get("utils.go")
	require.NotNil(t, utilsSel)

	require.Nil(t, m.Get("nonexistent.go"))

	// Test Contains.
	require.True(t, m.Contains("main.go", 15))
	require.True(t, m.Contains("main.go", 35))
	require.False(t, m.Contains("main.go", 25))
	require.True(t, m.Contains("utils.go", 7))
	require.False(t, m.Contains("other.go", 5))
}

func TestParseSelections(t *testing.T) {
	args := []string{"main.go:10-20", "utils.go:5,15-25"}

	selections, err := diff.ParseSelections(args)
	require.NoError(t, err)
	require.Len(t, selections, 2)

	require.Equal(t, "main.go", selections[0].Path)
	require.Equal(t, "utils.go", selections[1].Path)
}
