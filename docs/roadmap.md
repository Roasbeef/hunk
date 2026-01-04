# Roadmap

This document outlines potential improvements to make hunk even more effective for AI coding agents. These ideas emerged from real-world testing and thinking about the end-to-end agent workflow.

## The Agent Workflow Today

An agent using hunk follows this pattern:

1. Make edits to files
2. Run `hunk diff --json` to see all changes with line numbers
3. Parse the JSON to identify which lines correspond to the intended changes
4. Construct `hunk stage FILE:LINES` commands
5. Run `hunk commit -m "message"`

Each step has friction that could be reduced.

---

## Near-term Improvements

### Hunk-based Staging

The diff output naturally groups changes into hunks, but staging still requires specifying line numbers. Agents often want to stage "the entire second hunk" rather than calculating its line range.

```bash
# Current: must calculate line range from diff output
hunk stage main.go:42-58

# Proposed: reference hunks directly
hunk stage main.go:@1        # stage first hunk
hunk stage main.go:@2-3      # stage hunks 2 and 3
```

The JSON output could include a `hunk_id` field that's stable and referenceable:

```json
{
  "hunks": [
    {
      "id": 1,
      "header": "@@ -10,5 +10,8 @@",
      "lines": [...]
    }
  ]
}
```

This dramatically simplifies the agent's task: instead of parsing line numbers, it just says "stage hunk 2."

### Better Error Context

Current error messages are functional but could be more actionable:

```
# Current
Error: no matching lines found for selection

# Proposed
Error: no matching lines found for selection
  main.go:100-110 - file has changes only at lines 15-25, 42-48
  Hint: run 'hunk diff main.go' to see available line ranges
```

For agents, specific errors enable self-correction. Generic errors require guessing.

### Status Command

A dedicated status command would give agents a quick overview:

```bash
$ hunk status
Staged:
  main.go: 3 hunks, +15/-8 lines

Unstaged:
  utils.go: 1 hunk, +5/-2 lines

Untracked: 2 file(s)

$ hunk status --json
{
  "staged": {"files": 1, "additions": 15, "deletions": 8},
  "unstaged": {"files": 1, "additions": 5, "deletions": 2},
  "untracked": ["new.go", "other.go"]
}
```

This saves agents from parsing the full diff just to understand the current state.

### Expanded Context

The default 3 lines of context sometimes isn't enough to understand where a change sits:

```bash
hunk diff --context=10 main.go
```

More context helps agents verify they're staging the right thing, especially when changes are near similar-looking code.

---

## Medium-term Improvements

### Symbol-based Staging

Agents think in terms of the code they modified: "I changed the `processRequest` function." Translating that to line numbers requires parsing the diff. What if hunk could do it?

```bash
# Stage all changes within a function
hunk stage main.go:func:processRequest

# Stage all changes to a type definition
hunk stage main.go:type:Config

# Stage all changes in a specific block
hunk stage main.go:block:init
```

This requires language-aware parsing (tree-sitter or similar), but the payoff is significant. Agents could stage by semantic intent rather than mechanical line ranges.

Implementation approach:
1. Use tree-sitter for Go, Python, TypeScript, etc.
2. Map line ranges from diff to AST nodes
3. Allow staging by qualified symbol name

### Inverse Selection

Sometimes it's easier to specify what NOT to stage:

```bash
# Stage everything except lines 15-20
hunk stage main.go --exclude :15-20

# Stage all changes except those matching a pattern
hunk stage main.go --exclude-pattern "debug"
```

Useful when an agent made one exploratory change among many intentional ones.

### Atomic Multi-file Staging

Currently, if staging fails partway through a multi-file operation, some files are staged and others aren't:

```bash
hunk stage main.go:10-20 utils.go:invalid
# main.go changes are staged, utils.go fails
```

An atomic mode would roll back on any failure:

```bash
hunk stage --atomic main.go:10-20 utils.go:5-10
# Either all succeed or none are staged
```

### Staging from Patch Input

Agents sometimes generate patches programmatically. Currently they must write to a file:

```bash
echo "$PATCH" > /tmp/patch.diff
hunk apply-patch /tmp/patch.diff
```

Direct stdin support would be cleaner:

```bash
echo "$PATCH" | hunk apply-patch -
```

(This may already work, but should be documented and tested.)

### Batch Operations

For complex staging scenarios, reading from a file:

```bash
# selections.txt
main.go:10-20,30-40
utils.go:5-15
config.go:@1

# Apply all at once
hunk stage --from-file selections.txt
```

Useful for agents that plan their staging in advance.

---

## Longer-term Ideas

### Staging Sessions

Track staging operations as a session that can be rolled back entirely:

```bash
hunk session start

hunk stage main.go:10-20
hunk stage utils.go:5-10
# Oops, made a mistake

hunk session rollback  # Undo all staging since session start
# Or
hunk session commit -m "feature: add validation"
```

This gives agents a transaction-like model for complex multi-step staging.

### Validation Mode

Before committing, verify the staged changes don't break the build:

```bash
hunk stage --validate main.go:10-20
# Stages, runs 'go build', rolls back if it fails
```

Or as a separate verification step:

```bash
hunk validate
# Stashes unstaged changes, runs build/test, reports result
```

Configuration for what "validate" means could live in `.hunk.yaml` or be passed as a flag.

### Change Fingerprinting

Agents working across sessions need to track which changes are "theirs":

```bash
$ hunk diff --fingerprint
main.go:10-20  sha256:a3f2b1c...
main.go:30-35  sha256:7d8e9f0...
utils.go:5-10  sha256:1b2c3d4...
```

An agent could store these fingerprints and later verify:

```bash
$ hunk verify-fingerprint a3f2b1c
main.go:10-20 - still present and unchanged
```

### Pattern-based Staging

Stage changes matching a content pattern:

```bash
# Stage all additions containing "error"
hunk stage main.go --grep "error"

# Stage additions matching a regex
hunk stage main.go --grep-regex "^\\s*return err"
```

Useful when an agent knows the content of their changes but not the exact line numbers.

### Diff Against Specific Base

Compare against something other than the index:

```bash
# Changes since specific commit
hunk diff --base HEAD~3

# Changes between branches
hunk diff --base main --head feature-branch
```

Useful for agents working on feature branches.

### Explain Mode

For verification, explain what changes would do:

```bash
$ hunk explain main.go:10-20
Lines 10-20 in main.go:
  - Adds nil check before calling process()
  - Within function: handleRequest
  - Affects: error handling path
```

This requires understanding the code semantically, potentially via an LLM or static analysis.

---

## Non-goals

Some things explicitly out of scope:

**Interactive TUI**: Hunk is designed for programmatic use. Adding ncurses-style interfaces would add complexity without helping agents.

**Merge conflict resolution**: Hunk operates on the working tree and index. Merge conflicts are a different problem space.

**Remote operations**: Hunk is local-only. Pushing, pulling, and remote management are handled by git directly.

**Configuration files**: Keeping hunk zero-config means fewer things for agents to manage. Any "configuration" should be expressible as flags.

---

## Implementation Priority

If implementing these, suggested order:

1. **Hunk-based staging** (`main.go:@1`) - High impact, moderate complexity
2. **Status command** - High impact, low complexity
3. **Better error messages** - High impact, low complexity
4. **Expanded context flag** - Low complexity
5. **Symbol-based staging** - Very high impact, high complexity (needs tree-sitter)
6. **Atomic mode** - Moderate impact, moderate complexity
7. **Sessions** - Moderate impact, moderate complexity

The goal is always: reduce friction between what an agent intends and what hunk does.
