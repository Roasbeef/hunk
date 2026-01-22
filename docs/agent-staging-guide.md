# Agent Staging Guide

This guide explains how AI agents can use hunk for precision line-level staging and atomic commits. It covers the complete workflow from viewing changes to committing, with emphasis on machine-readable output and error handling.

## Why Line-Level Staging?

The problem with `git add -A` or `git add .` is that agents often make changes across multiple files but need to commit them in logical, focused units. When an agent modifies three different features in a single file, staging the entire file produces unfocused commits that are harder to review and revert.

Hunk solves this by letting agents stage exactly the lines they modified. The agent knows precisely which lines correspond to each logical change, and hunk provides a deterministic interface to stage those specific lines without interactive prompts.

**The core insight**: agents don't need to interactively review hunks or make judgment calls about context. They need a programmatic way to say "stage these specific lines" and have it work reliably.

## Core Workflow

The typical agent workflow follows this pattern:

```
diff → stage → preview → commit
```

1. **diff**: View all changes with line numbers (use `--json` for parsing).
2. **stage**: Stage specific lines using `FILE:LINES` syntax.
3. **preview**: Verify the staged changes are correct.
4. **commit**: Create the commit.

If staging produces unexpected results, use `reset` to unstage and try again.

### Step-by-Step Example

```bash
# 1. See what changed
hunk diff --json

# 2. Stage specific lines
hunk stage main.go:42-58

# 3. Verify what will be committed
hunk preview --json

# 4. Commit if preview looks correct
hunk commit -m "fix: handle nil pointer in processRequest"

# If something went wrong, reset and try again
hunk reset
```

## FILE:LINES Syntax

The staging syntax is designed for programmatic construction. It follows the pattern `FILE:LINES` where LINES is a comma-separated list of line numbers or ranges.

### Syntax Reference

| Syntax | Description | Example |
|--------|-------------|---------|
| `file:N` | Single line | `main.go:42` |
| `file:N-M` | Range (inclusive) | `main.go:10-20` |
| `file:N-M,X-Y` | Multiple ranges | `main.go:10-20,30-40` |
| `file:N,M,X` | Individual lines | `main.go:10,15,20` |
| `file:N-M,X` | Mixed | `main.go:10-20,30` |

Multiple files are space-separated arguments:

```bash
hunk stage main.go:10-20 utils.go:5-8 config.go:100
```

### Important: Line Numbers Refer to New File

Line numbers in hunk always refer to the **new file** (after edits), not the old file. This matches what editors display—if your editor shows line 42, use line 42 in hunk.

This is critical for agents: when you edit a file, the line numbers you see in your editor or in the diff output are the ones to use.

## JSON Output

For reliable parsing, always use `--json` when calling hunk from an agent.

### hunk diff --json

Returns all unstaged changes with full line number information.

```json
{
  "files": [
    {
      "path": "main.go",
      "status": "modified",
      "hunks": [
        {
          "header": "@@ -10,5 +10,8 @@",
          "section": "func processRequest",
          "lines": [
            {
              "op": "context",
              "content": "func processRequest(r *Request) error {",
              "old_line": 10,
              "new_line": 10
            },
            {
              "op": "add",
              "content": "\tif r == nil {",
              "new_line": 11
            },
            {
              "op": "add",
              "content": "\t\treturn ErrNilRequest",
              "new_line": 12
            },
            {
              "op": "add",
              "content": "\t}",
              "new_line": 13
            },
            {
              "op": "context",
              "content": "\treturn r.Process()",
              "old_line": 11,
              "new_line": 14
            }
          ]
        }
      ]
    }
  ],
  "untracked": ["new_file.go"]
}
```

**Field Reference**:

| Field | Type | Description |
|-------|------|-------------|
| `files` | array | List of modified files |
| `files[].path` | string | File path relative to repo root |
| `files[].old_path` | string | Original path if renamed (omitted otherwise) |
| `files[].status` | string | One of: `modified`, `new`, `deleted`, `renamed` |
| `files[].binary` | boolean | True if binary file (omitted if false) |
| `files[].hunks` | array | List of change hunks |
| `hunks[].header` | string | Unified diff header (e.g., `@@ -10,5 +10,8 @@`) |
| `hunks[].section` | string | Function/section name if available |
| `hunks[].lines` | array | Lines in the hunk |
| `lines[].op` | string | One of: `add`, `delete`, `context` |
| `lines[].content` | string | Line content (without +/- prefix) |
| `lines[].old_line` | integer | Line number in old file (context/delete only) |
| `lines[].new_line` | integer | Line number in new file (context/add only) |
| `untracked` | array | List of untracked file paths |

**Extracting Stageable Lines**:

To construct a stage command from JSON output, collect `new_line` values from lines where `op` is `add`:

```python
# Pseudocode for extracting stageable lines
for file in output["files"]:
    lines_to_stage = []
    for hunk in file["hunks"]:
        for line in hunk["lines"]:
            if line["op"] == "add":
                lines_to_stage.append(line["new_line"])

    # Convert to ranges for efficiency
    ranges = compress_to_ranges(lines_to_stage)
    command = f"hunk stage {file['path']}:{ranges}"
```

### hunk preview --json

Returns currently staged changes in the same format as `hunk diff --json`, but without the `untracked` field.

```json
{
  "files": [
    {
      "path": "main.go",
      "status": "modified",
      "hunks": [...]
    }
  ]
}
```

Use `preview --json` to verify that staging captured exactly the intended changes before committing.

### Empty Results

When there are no changes, the output is:

```json
{
  "files": [],
  "untracked": []
}
```

## Complete Example Session

This example shows an agent making changes, staging specific parts, and committing.

```bash
# Agent has made changes to two files:
# - main.go: added error handling (lines 42-50) and logging (lines 100-105)
# - utils.go: fixed a bug (lines 15-20)

# Step 1: Review all changes
$ hunk diff --json
{
  "files": [
    {
      "path": "main.go",
      "status": "modified",
      "hunks": [
        {
          "header": "@@ -40,5 +40,13 @@",
          "lines": [
            {"op": "context", "content": "func handleRequest(w http.ResponseWriter, r *http.Request) {", "old_line": 40, "new_line": 40},
            {"op": "context", "content": "\tctx := r.Context()", "old_line": 41, "new_line": 41},
            {"op": "add", "content": "\tif r.Body == nil {", "new_line": 42},
            {"op": "add", "content": "\t\thttp.Error(w, \"empty body\", 400)", "new_line": 43},
            {"op": "add", "content": "\t\treturn", "new_line": 44},
            {"op": "add", "content": "\t}", "new_line": 45}
          ]
        },
        {
          "header": "@@ -98,3 +106,8 @@",
          "lines": [
            {"op": "context", "content": "func processPayload(data []byte) error {", "old_line": 98, "new_line": 106},
            {"op": "add", "content": "\tlog.Printf(\"processing %d bytes\", len(data))", "new_line": 107}
          ]
        }
      ]
    },
    {
      "path": "utils.go",
      "status": "modified",
      "hunks": [
        {
          "header": "@@ -13,5 +13,8 @@",
          "lines": [
            {"op": "delete", "content": "\treturn strings.Split(s, \",\")", "old_line": 15},
            {"op": "add", "content": "\tif s == \"\" {", "new_line": 15},
            {"op": "add", "content": "\t\treturn nil", "new_line": 16},
            {"op": "add", "content": "\t}", "new_line": 17},
            {"op": "add", "content": "\treturn strings.Split(s, \",\")", "new_line": 18}
          ]
        }
      ]
    }
  ]
}

# Step 2: Create first commit - error handling in main.go
$ hunk stage main.go:42-45

# Step 3: Verify staged changes
$ hunk preview --json
{
  "files": [
    {
      "path": "main.go",
      "status": "modified",
      "hunks": [
        {
          "header": "@@ -40,5 +40,9 @@",
          "lines": [
            {"op": "context", "content": "func handleRequest(w http.ResponseWriter, r *http.Request) {", "old_line": 40, "new_line": 40},
            {"op": "context", "content": "\tctx := r.Context()", "old_line": 41, "new_line": 41},
            {"op": "add", "content": "\tif r.Body == nil {", "new_line": 42},
            {"op": "add", "content": "\t\thttp.Error(w, \"empty body\", 400)", "new_line": 43},
            {"op": "add", "content": "\t\treturn", "new_line": 44},
            {"op": "add", "content": "\t}", "new_line": 45}
          ]
        }
      ]
    }
  ]
}

# Step 4: Commit error handling
$ hunk commit -m "fix: return 400 on empty request body"
Committed successfully.

# Step 5: Stage and commit the bug fix
$ hunk stage utils.go:15-18
$ hunk preview
# (verify it looks right)
$ hunk commit -m "fix: handle empty string in parseCSV"
Committed successfully.

# Step 6: Stage and commit the logging addition
$ hunk stage main.go:107
$ hunk commit -m "chore: add logging to processPayload"
Committed successfully.
```

The result is three focused commits instead of one monolithic commit containing unrelated changes.

## Error Handling

Hunk returns non-zero exit codes on errors. Common error scenarios:

### No Unstaged Changes

```bash
$ hunk stage main.go:10-20
Error: no unstaged changes
```

**Cause**: The working tree has no modifications, or all changes are already staged.

**Recovery**: Check with `hunk diff` or `git status` to understand the current state.

### No Matching Lines

```bash
$ hunk stage main.go:100-110
Error: no matching lines found for selection
```

**Cause**: The specified lines don't correspond to any additions in the diff. This can happen if:
- The line numbers are wrong.
- The changes at those lines were already staged.
- The lines contain only context or deletions, not additions.

**Recovery**: Run `hunk diff main.go` to see the actual line ranges that have changes.

### Invalid Syntax

```bash
$ hunk stage main.go
Error: invalid selection: expected FILE:LINES, got "main.go"

$ hunk stage main.go:abc
Error: invalid selection: invalid range "abc"
```

**Cause**: The `FILE:LINES` syntax was malformed.

**Recovery**: Ensure the format is `file:N` or `file:N-M` where N and M are integers.

### Nothing Staged for Commit

```bash
$ hunk commit -m "message"
Error: nothing staged for commit
```

**Cause**: The staging area is empty.

**Recovery**: Stage changes first with `hunk stage`, or verify that previous staging succeeded.

### Patch Application Failure

```bash
$ hunk stage main.go:10-20
Error: failed to stage changes: patch does not apply
```

**Cause**: The patch generated from the selection couldn't be applied. This is rare but can happen if the working tree changed between `hunk diff` and `hunk stage`.

**Recovery**: Run `hunk diff` again to get fresh line numbers.

## Unstaging Changes

If staging produced unexpected results, use `hunk reset` to unstage:

```bash
# Unstage everything
hunk reset

# Unstage specific file
hunk reset main.go
```

After resetting, run `hunk diff` to see line numbers again and retry staging.

## Best Practices

### Always Parse JSON Programmatically

Don't parse text output with regex. The JSON format is stable and unambiguous.

```bash
# Good
hunk diff --json | jq '.files[].path'

# Fragile
hunk diff | grep "^---"
```

### Verify Before Committing

Always run `hunk preview` (or `hunk preview --json`) before committing to catch staging mistakes.

### Use Dry Run for Debugging

The `--dry-run` flag shows what patch would be applied without actually staging:

```bash
hunk stage --dry-run main.go:10-20
```

This outputs the unified diff patch that would be applied. Useful for debugging staging issues.

### Stage Related Changes Together

Group related changes in a single stage command:

```bash
# Stage all lines of a single logical change
hunk stage main.go:10-20,25-30 utils.go:50-55
```

### Keep Commits Focused

Make multiple small commits rather than one large commit. This makes code review easier and enables precise reverts if needed.

### Handle Untracked Files Separately

Hunk's staging operates on the diff of tracked files. For untracked (new) files, use `git add`:

```bash
# Add new file
git add new_file.go

# Stage changes to existing files
hunk stage existing.go:10-20
```

The `untracked` field in JSON output lists files that need `git add`.

## Integration Tips

### Working Directory

Use the `--dir` flag to operate on a repository that isn't the current directory:

```bash
hunk --dir /path/to/repo diff --json
```

### Error Checking

Check exit codes for all hunk commands:

```python
result = subprocess.run(["hunk", "stage", "main.go:10-20"], capture_output=True)
if result.returncode != 0:
    error_message = result.stderr.decode()
    # Handle error
```

### Idempotency

Staging is idempotent—staging the same lines twice won't cause errors, but it also won't stage them again if they're already staged. The second attempt will return "no matching lines found" if all specified lines are already staged.

### Atomic Operations

If you need multiple files to be staged atomically (all or nothing), hunk doesn't currently support this directly. For now, check results after each stage and `hunk reset` on failure.
