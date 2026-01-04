package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func TestLineOp_String(t *testing.T) {
	tests := []struct {
		op   diff.LineOp
		want string
	}{
		{diff.OpContext, "context"},
		{diff.OpAdd, "add"},
		{diff.OpDelete, "delete"},
		{diff.LineOp(99), "unknown"},
	}

	for _, tc := range tests {
		require.Equal(t, tc.want, tc.op.String())
	}
}

func TestLineOp_Prefix(t *testing.T) {
	tests := []struct {
		op   diff.LineOp
		want byte
	}{
		{diff.OpContext, ' '},
		{diff.OpAdd, '+'},
		{diff.OpDelete, '-'},
		{diff.LineOp(99), ' '}, // Default to space.
	}

	for _, tc := range tests {
		require.Equal(t, tc.want, tc.op.Prefix())
	}
}

func TestDiffLine_String(t *testing.T) {
	tests := []struct {
		name string
		line diff.DiffLine
		want string
	}{
		{
			name: "context line",
			line: diff.DiffLine{Op: diff.OpContext, Content: "unchanged"},
			want: " unchanged",
		},
		{
			name: "added line",
			line: diff.DiffLine{Op: diff.OpAdd, Content: "new line"},
			want: "+new line",
		},
		{
			name: "deleted line",
			line: diff.DiffLine{Op: diff.OpDelete, Content: "old line"},
			want: "-old line",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.line.String())
		})
	}
}

func TestDiffLine_LineRef(t *testing.T) {
	tests := []struct {
		name string
		line diff.DiffLine
		want string
	}{
		{
			name: "context line",
			line: diff.DiffLine{
				Op: diff.OpContext, OldLineNum: 10, NewLineNum: 12,
			},
			want: "10:12",
		},
		{
			name: "added line",
			line: diff.DiffLine{
				Op: diff.OpAdd, OldLineNum: 0, NewLineNum: 15,
			},
			want: "-:15",
		},
		{
			name: "deleted line",
			line: diff.DiffLine{
				Op: diff.OpDelete, OldLineNum: 20, NewLineNum: 0,
			},
			want: "20:-",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.line.LineRef())
		})
	}
}

func TestDiffLine_IsChange(t *testing.T) {
	tests := []struct {
		op   diff.LineOp
		want bool
	}{
		{diff.OpContext, false},
		{diff.OpAdd, true},
		{diff.OpDelete, true},
	}

	for _, tc := range tests {
		line := diff.DiffLine{Op: tc.op}
		require.Equal(t, tc.want, line.IsChange())
	}
}

func TestDiffLine_EffectiveLineNum(t *testing.T) {
	tests := []struct {
		name string
		line diff.DiffLine
		want int
	}{
		{
			name: "context uses old",
			line: diff.DiffLine{
				Op: diff.OpContext, OldLineNum: 10, NewLineNum: 12,
			},
			want: 10,
		},
		{
			name: "add uses new",
			line: diff.DiffLine{
				Op: diff.OpAdd, OldLineNum: 0, NewLineNum: 15,
			},
			want: 15,
		},
		{
			name: "delete uses old",
			line: diff.DiffLine{
				Op: diff.OpDelete, OldLineNum: 20, NewLineNum: 0,
			},
			want: 20,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.line.EffectiveLineNum())
		})
	}
}

func TestDiffLine_Format(t *testing.T) {
	tests := []struct {
		name string
		line diff.DiffLine
		want string
	}{
		{
			name: "context line",
			line: diff.DiffLine{
				Op:         diff.OpContext,
				Content:    "code",
				OldLineNum: 10,
				NewLineNum: 10,
			},
			want: "  10   10  code",
		},
		{
			name: "added line",
			line: diff.DiffLine{
				Op:         diff.OpAdd,
				Content:    "new",
				OldLineNum: 0,
				NewLineNum: 15,
			},
			want: "       15 +new",
		},
		{
			name: "deleted line",
			line: diff.DiffLine{
				Op:         diff.OpDelete,
				Content:    "old",
				OldLineNum: 20,
				NewLineNum: 0,
			},
			want: "  20      -old",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.line.Format())
		})
	}
}
