package diff_test

import (
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  bool
		validate func(t *testing.T, d *diff.ParsedDiff)
	}{
		{
			name:  "empty diff",
			input: "",
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				require.Equal(t, 0, d.FileCount())
			},
		},
		{
			name:  "whitespace only",
			input: "   \n\t\n  ",
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				require.Equal(t, 0, d.FileCount())
			},
		},
		{
			name: "simple add",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// New comment.
 func main() {}
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				require.Equal(t, 1, d.FileCount())

				var files []*diff.FileDiff
				for f := range d.Files() {
					files = append(files, f)
				}

				require.Len(t, files, 1)
				require.Equal(t, "main.go", files[0].Path())
				require.Len(t, files[0].Hunks, 1)

				hunk := files[0].Hunks[0]
				require.GreaterOrEqual(t, len(hunk.Lines), 3)

				// Find the added line.
				var addLine diff.DiffLine
				for _, line := range hunk.Lines {
					if line.Op == diff.OpAdd {
						addLine = line

						break
					}
				}

				require.Equal(t, diff.OpAdd, addLine.Op)
				require.Equal(t, "// New comment.", addLine.Content)
				require.Positive(t, addLine.NewLineNum)
				require.Equal(t, 0, addLine.OldLineNum)
			},
		},
		{
			name: "simple delete",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,4 +1,3 @@
 package main
-// Old comment.
 func main() {}
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				require.Equal(t, 1, d.FileCount())

				files := d.AllFiles()
				require.Len(t, files, 1)

				hunk := files[0].Hunks[0]

				// Find the deleted line.
				var delLine diff.DiffLine
				for _, line := range hunk.Lines {
					if line.Op == diff.OpDelete {
						delLine = line

						break
					}
				}

				require.Equal(t, "// Old comment.", delLine.Content)
				require.Positive(t, delLine.OldLineNum)
				require.Equal(t, 0, delLine.NewLineNum)
			},
		},
		{
			name: "modification",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,3 @@
 package main

-func main() {}
+func main() { println("hello") }
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				added, deleted := d.Stats()
				require.Equal(t, 1, added)
				require.Equal(t, 1, deleted)
			},
		},
		{
			name: "multiple hunks",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main

+import "fmt"
 func main() {}
@@ -10,3 +11,4 @@
 func helper() {
     return
 }
+// End of file.
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				files := d.AllFiles()
				require.Len(t, files, 1)
				require.Len(t, files[0].Hunks, 2)

				require.Equal(t, 1, files[0].Hunks[0].OldStart)
				require.Equal(t, 10, files[0].Hunks[1].OldStart)
			},
		},
		{
			name: "multiple files",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main

+import "fmt"
 func main() {}
diff --git a/utils.go b/utils.go
--- a/utils.go
+++ b/utils.go
@@ -1,2 +1,3 @@
 package main

+func helper() {}
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				require.Equal(t, 2, d.FileCount())

				main := d.FileByPath("main.go")
				require.NotNil(t, main)

				utils := d.FileByPath("utils.go")
				require.NotNil(t, utils)
			},
		},
		{
			name: "new file",
			input: `diff --git a/newfile.go b/newfile.go
new file mode 100644
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+package main
+
+func newFunc() {}
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				files := d.AllFiles()
				require.Len(t, files, 1)
				require.True(t, files[0].IsNew)
				require.Equal(t, "newfile.go", files[0].Path())
			},
		},
		{
			name: "deleted file",
			input: `diff --git a/oldfile.go b/oldfile.go
deleted file mode 100644
--- a/oldfile.go
+++ /dev/null
@@ -1,3 +0,0 @@
-package main
-
-func oldFunc() {}
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				files := d.AllFiles()
				require.Len(t, files, 1)
				require.True(t, files[0].IsDeleted)
				require.Equal(t, "oldfile.go", files[0].Path())
			},
		},
		{
			name: "with section header",
			input: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -10,6 +10,7 @@ func handler() {
     if err != nil {
         return err
     }
+    log.Println("success")
     return nil
 }
`,
			validate: func(t *testing.T, d *diff.ParsedDiff) {
				files := d.AllFiles()
				require.Len(t, files, 1)

				hunk := files[0].Hunks[0]
				require.Equal(t, "func handler() {", hunk.Section)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, err := diff.Parse(tc.input)
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			tc.validate(t, d)
		})
	}
}

func TestLineNumbers(t *testing.T) {
	input := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -5,5 +5,7 @@ package main
 import "fmt"
 func main() {
-    fmt.Println("old")
+    // Added comment.
+    fmt.Println("new")
+    fmt.Println("also new")
 }
`

	d, err := diff.Parse(input)
	require.NoError(t, err)

	files := d.AllFiles()
	require.Len(t, files, 1)

	hunk := files[0].Hunks[0]

	// First line should be context at old:5, new:5.
	require.Equal(t, diff.OpContext, hunk.Lines[0].Op)
	require.Equal(t, 5, hunk.Lines[0].OldLineNum)
	require.Equal(t, 5, hunk.Lines[0].NewLineNum)

	// Find the deleted line.
	var delLine diff.DiffLine
	for _, line := range hunk.Lines {
		if line.Op == diff.OpDelete {
			delLine = line

			break
		}
	}

	require.Positive(t, delLine.OldLineNum)
	require.Equal(t, 0, delLine.NewLineNum)

	// First added line.
	var addLine diff.DiffLine
	for _, line := range hunk.Lines {
		if line.Op == diff.OpAdd {
			addLine = line

			break
		}
	}

	require.Equal(t, 0, addLine.OldLineNum)
	require.Positive(t, addLine.NewLineNum)
}

func TestLinesWithContext(t *testing.T) {
	input := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Comment.
 func main() {}
`

	d, err := diff.Parse(input)
	require.NoError(t, err)

	var contexts []diff.LineWithContext
	for ctx := range d.LinesWithContext() {
		contexts = append(contexts, ctx)
	}

	require.GreaterOrEqual(t, len(contexts), 3)

	// Find the added line.
	var addCtx diff.LineWithContext
	for _, ctx := range contexts {
		if ctx.Line.Op == diff.OpAdd {
			addCtx = ctx

			break
		}
	}

	require.Equal(t, diff.OpAdd, addCtx.Line.Op)
	require.Equal(t, 0, addCtx.HunkIndex)
	require.Equal(t, "main.go", addCtx.File.Path())
}
