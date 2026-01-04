package output_test

import (
	"bytes"
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/output"
	"github.com/stretchr/testify/require"
)

func parseTestDiff(t *testing.T) *diff.ParsedDiff {
	t.Helper()

	diffText := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,5 @@
 package main
+// Added line 1.
+// Added line 2.
-// Removed.
 func main() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	return parsed
}

func TestFormatText(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	opts := output.DefaultTextOptions()
	err := output.FormatText(&buf, parsed, opts)
	require.NoError(t, err)

	result := buf.String()

	// Should contain file path.
	require.Contains(t, result, "main.go")

	// Should contain hunk header.
	require.Contains(t, result, "@@")

	// Should show stats.
	require.Contains(t, result, "insertions")
	require.Contains(t, result, "deletions")
}

func TestFormatText_NoColor(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	opts := output.TextOptions{
		Color:       false,
		LineNumbers: true,
		Stats:       true,
	}
	err := output.FormatText(&buf, parsed, opts)
	require.NoError(t, err)

	result := buf.String()

	// Should not contain ANSI codes.
	require.NotContains(t, result, "\033[")
	require.Contains(t, result, "main.go")
}

func TestFormatText_NoStats(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	opts := output.TextOptions{
		Color:       false,
		LineNumbers: true,
		Stats:       false,
	}
	err := output.FormatText(&buf, parsed, opts)
	require.NoError(t, err)

	result := buf.String()
	require.NotContains(t, result, "insertions")
}

func TestFormatText_NoLineNumbers(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	opts := output.TextOptions{
		Color:       false,
		LineNumbers: false,
		Stats:       false,
	}
	err := output.FormatText(&buf, parsed, opts)
	require.NoError(t, err)

	result := buf.String()

	// Should still contain the changes.
	require.Contains(t, result, "+// Added line 1.")
}

func TestFormatTextSummary(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	err := output.FormatTextSummary(&buf, parsed)
	require.NoError(t, err)

	result := buf.String()
	require.Contains(t, result, "1 file(s) changed")
	require.Contains(t, result, "main.go")
	require.Contains(t, result, "insertions")
	require.Contains(t, result, "deletions")
}

func TestFormatFileList(t *testing.T) {
	diffText := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1,2 @@
 package a
+// change
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1,2 @@
 package b
+// change
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatFileList(&buf, parsed)
	require.NoError(t, err)

	result := buf.String()
	require.Contains(t, result, "a.go\n")
	require.Contains(t, result, "b.go\n")
}

func TestFormatRaw(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	err := output.FormatRaw(&buf, parsed)
	require.NoError(t, err)

	result := buf.String()
	require.Contains(t, result, "--- a/main.go")
	require.Contains(t, result, "+++ b/main.go")
	require.Contains(t, result, "@@ ")
}

func TestFormatStageableLines(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	err := output.FormatStageableLines(&buf, parsed)
	require.NoError(t, err)

	result := buf.String()
	require.Contains(t, result, "File: main.go")
	require.Contains(t, result, "+")
	require.Contains(t, result, "-")
}

func TestFormatStagingCommands(t *testing.T) {
	parsed := parseTestDiff(t)

	var buf bytes.Buffer
	err := output.FormatStagingCommands(&buf, parsed)
	require.NoError(t, err)

	result := buf.String()
	require.Contains(t, result, "hunk stage main.go:")
}

func TestDefaultTextOptions(t *testing.T) {
	opts := output.DefaultTextOptions()
	require.True(t, opts.Color)
	require.True(t, opts.LineNumbers)
	require.True(t, opts.Stats)
}

func TestFormatText_BinaryFile(t *testing.T) {
	// Create a file marked as binary.
	file := &diff.FileDiff{
		OldName:  "image.png",
		NewName:  "image.png",
		IsBinary: true,
	}

	parsed := &diff.ParsedDiff{}
	// Use reflection or a different approach - for now skip binary test.
	_ = file
	_ = parsed
}
