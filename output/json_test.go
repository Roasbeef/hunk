package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/output"
	"github.com/stretchr/testify/require"
)

func TestFormatJSON(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Added comment.
 func main() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	// Verify it's valid JSON.
	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	require.Equal(t, "main.go", result.Files[0].Path)
	require.Equal(t, "modified", result.Files[0].Status)
	require.Len(t, result.Files[0].Hunks, 1)
	require.GreaterOrEqual(t, len(result.Files[0].Hunks[0].Hunks), 2)
}

func TestFormatJSON_NewFile(t *testing.T) {
	diffText := `diff --git a/newfile.go b/newfile.go
new file mode 100644
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+package main
+
+func newFunc() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	require.Equal(t, "new", result.Files[0].Status)
}

func TestFormatJSON_DeletedFile(t *testing.T) {
	diffText := `diff --git a/oldfile.go b/oldfile.go
deleted file mode 100644
--- a/oldfile.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func oldFunc() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	require.Equal(t, "deleted", result.Files[0].Status)
}

func TestFormatJSON_RenamedFile(t *testing.T) {
	diffText := `diff --git a/old.go b/new.go
similarity index 90%
rename from old.go
rename to new.go
--- a/old.go
+++ b/new.go
@@ -1,3 +1,3 @@
 package main
-// old
+// new
 func main() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result.Files, 1)
	require.Equal(t, "renamed", result.Files[0].Status)
	require.Equal(t, "new.go", result.Files[0].Path)
	require.Equal(t, "old.go", result.Files[0].OldPath)
}

func TestFormatJSON_MultipleFiles(t *testing.T) {
	diffText := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1,2 @@
 package a
+// change a
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +1,2 @@
 package b
+// change b
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	require.Len(t, result.Files, 2)
	require.Equal(t, "a.go", result.Files[0].Path)
	require.Equal(t, "b.go", result.Files[1].Path)
}

func TestFormatJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := output.FormatJSONEmpty(&buf)
	require.NoError(t, err)

	// Should produce valid JSON with empty files array.
	require.Contains(t, buf.String(), "\"files\": []")
}

func TestFormatJSONEmptyWithUntracked(t *testing.T) {
	var buf bytes.Buffer
	err := output.FormatJSONEmptyWithUntracked(&buf, []string{"new.go", "other.go"})
	require.NoError(t, err)

	require.Contains(t, buf.String(), "\"files\": []")
	require.Contains(t, buf.String(), "\"untracked\"")
	require.Contains(t, buf.String(), "new.go")
	require.Contains(t, buf.String(), "other.go")
}

func TestFormatJSON_LineNumbers(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -5,3 +5,4 @@
 func main() {
+    // added
 }
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = output.FormatJSON(&buf, parsed)
	require.NoError(t, err)

	var result output.DiffOutput
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	// Find the added line.
	var addLine output.LineOutput
	for _, l := range result.Files[0].Hunks[0].Hunks {
		if l.Op == "add" {
			addLine = l

			break
		}
	}

	require.Equal(t, "add", addLine.Op)
	require.Equal(t, "    // added", addLine.Content)
	require.Greater(t, addLine.NewLineNum, 0)
}
