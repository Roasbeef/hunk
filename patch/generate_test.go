package patch_test

import (
	"strings"
	"testing"

	"github.com/roasbeef/hunk/diff"
	"github.com/roasbeef/hunk/patch"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name       string
		diffText   string
		selections []string
		wantEmpty  bool
		validate   func(t *testing.T, result []byte)
	}{
		{
			name: "select single added line",
			diffText: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,5 @@
 package main
+// Added line 1.
+// Added line 2.
 func main() {}
`,
			selections: []string{"main.go:2"},
			validate: func(t *testing.T, result []byte) {
				s := string(result)
				require.Contains(t, s, "+// Added line 1.")
				require.NotContains(t, s, "+// Added line 2.")
			},
		},
		{
			name: "select range of lines",
			diffText: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,6 @@
 package main
+// Line 2.
+// Line 3.
+// Line 4.
 func main() {}
`,
			selections: []string{"main.go:2-3"},
			validate: func(t *testing.T, result []byte) {
				s := string(result)
				require.Contains(t, s, "+// Line 2.")
				require.Contains(t, s, "+// Line 3.")
				require.NotContains(t, s, "+// Line 4.")
			},
		},
		{
			name: "no matching lines",
			diffText: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Added.
 func main() {}
`,
			selections: []string{"main.go:100-200"},
			wantEmpty:  true,
		},
		{
			name: "multiple files",
			diffText: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Main change.
 func main() {}
diff --git a/utils.go b/utils.go
--- a/utils.go
+++ b/utils.go
@@ -1,3 +1,4 @@
 package main
+// Utils change.
 func helper() {}
`,
			selections: []string{"main.go:2", "utils.go:2"},
			validate: func(t *testing.T, result []byte) {
				s := string(result)
				require.Contains(t, s, "--- a/main.go")
				require.Contains(t, s, "+// Main change.")
				require.Contains(t, s, "--- a/utils.go")
				require.Contains(t, s, "+// Utils change.")
			},
		},
		{
			name: "select only from one file",
			diffText: `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Main change.
 func main() {}
diff --git a/utils.go b/utils.go
--- a/utils.go
+++ b/utils.go
@@ -1,3 +1,4 @@
 package main
+// Utils change.
 func helper() {}
`,
			selections: []string{"main.go:2"},
			validate: func(t *testing.T, result []byte) {
				s := string(result)
				require.Contains(t, s, "--- a/main.go")
				require.NotContains(t, s, "--- a/utils.go")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := diff.Parse(tc.diffText)
			require.NoError(t, err)

			selections, err := diff.ParseSelections(tc.selections)
			require.NoError(t, err)

			result, err := patch.Generate(parsed, selections)
			require.NoError(t, err)

			if tc.wantEmpty {
				require.Empty(t, result)

				return
			}

			require.NotEmpty(t, result)

			if tc.validate != nil {
				tc.validate(t, result)
			}

			// Verify it's valid unified diff format.
			verifyValidPatch(t, result)
		})
	}
}

func verifyValidPatch(t *testing.T, patchBytes []byte) {
	t.Helper()

	s := string(patchBytes)

	// Should have file headers.
	require.Contains(t, s, "--- a/")
	require.Contains(t, s, "+++ b/")

	// Should have at least one hunk header.
	require.Contains(t, s, "@@")

	// Line counts in hunk header should be valid.
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			// Verify format: @@ -X,Y +X,Y @@
			require.Contains(t, line, "-")
			require.Contains(t, line, "+")
		}
	}
}

func TestGenerateForFile(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// Added.
 func main() {}
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	files := parsed.AllFiles()
	require.Len(t, files, 1)

	result := patch.GenerateForFile(files[0])
	require.NotEmpty(t, result)

	s := string(result)
	require.Contains(t, s, "--- a/main.go")
	require.Contains(t, s, "+++ b/main.go")
	require.Contains(t, s, "+// Added.")
}

func TestGenerateForHunk(t *testing.T) {
	diffText := `diff --git a/main.go b/main.go
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+// First hunk.
 func main() {}
@@ -10,3 +11,4 @@
 func helper() {
+    // Second hunk.
 }
`

	parsed, err := diff.Parse(diffText)
	require.NoError(t, err)

	files := parsed.AllFiles()
	require.Len(t, files, 1)
	require.Len(t, files[0].Hunks, 2)

	// Generate patch for just the first hunk.
	result := patch.GenerateForHunk(files[0], files[0].Hunks[0])
	require.NotEmpty(t, result)

	s := string(result)
	require.Contains(t, s, "+// First hunk.")
	require.NotContains(t, s, "+    // Second hunk.")
}
