# hunk

Hunk is a command-line tool for making precise, line-level partial commits in git. It was designed specifically for AI coding agents that need to stage surgical changes to a codebase without committing unrelated modifications that happen to be in the same file.

## The Problem

When an AI agent edits code, it typically knows exactly which lines it changed. The agent might fix a bug on lines 42-45, but the file also contains unrelated uncommitted changes from earlier work. Git's built-in `add -p` is interactive and designed for humans—it prompts, waits for input, and operates at hunk granularity rather than line granularity.

This creates friction. Agents end up either committing too much (the whole file) or constructing patches by hand, which is error-prone and tedious to generate correctly.

## The Solution

Hunk lets you stage changes by specifying line numbers directly:

```bash
hunk stage main.go:42-45
```

That's it. Lines 42 through 45 from `main.go` are now staged, and nothing else. The remaining unstaged changes in the file stay exactly where they are.

The line numbers refer to the **new file** (after your edits), which matches how editors display content and how agents think about their changes. You can see all available changes with line numbers using `hunk diff`:

```bash
$ hunk diff
main.go
  10   func processRequest(r *Request) error {
  11 +     if r == nil {
  12 +         return errors.New("nil request")
  13 +     }
  14       // existing code...
```

## Installation

```bash
go install github.com/roasbeef/hunk/cmd/hunk@latest
```

Or build from source:

```bash
git clone https://github.com/roasbeef/hunk.git
cd hunk
make build
```

## Usage

The workflow is straightforward. First, see what's changed:

```bash
hunk diff                    # show unstaged changes with line numbers
hunk diff --staged           # show what's already staged
hunk diff --json             # machine-readable output for agents
```

Then stage the specific lines you want:

```bash
hunk stage main.go:10-20            # stage lines 10-20
hunk stage main.go:10-20,30-40      # stage multiple ranges
hunk stage main.go:10 utils.go:5-8  # stage from multiple files
hunk stage --dry-run main.go:10-20  # preview the patch without staging
```

Check what you're about to commit:

```bash
hunk preview                 # show staged changes
hunk preview --raw           # show as unified diff
```

Commit when ready:

```bash
hunk commit -m "fix nil pointer in request handler"
```

And if you change your mind:

```bash
hunk reset                   # unstage everything
hunk reset main.go           # unstage just one file
```

## Interactive Rebase

Hunk also provides programmatic interactive rebase for agents. Traditional `git rebase -i` opens an editor—impossible for automated workflows. Hunk's rebase commands let you specify actions declaratively:

```bash
# See what commits would be rebased
hunk rebase list --onto main
hunk rebase list --onto main --json  # machine-readable

# Execute rebase with explicit actions
hunk rebase run --onto main abc123,def456           # pick all
hunk rebase run --onto main pick:abc123,squash:def456,drop:ghi789

# Handle conflicts
hunk rebase status              # check current state
hunk rebase continue            # after resolving conflicts
hunk rebase abort               # cancel the rebase
hunk rebase skip                # skip problematic commit
```

For complex operations, use JSON input:

```bash
echo '{"actions":[{"action":"pick","commit":"abc123"},{"action":"squash","commit":"def456"}]}' | \
  hunk rebase run --onto main --spec -
```

Supported actions: `pick`, `reword`, `squash`, `fixup`, `drop`, `edit`, `exec`.

## Why This Matters for Agents

Traditional git workflows assume a human is making decisions interactively. An agent, however, operates programmatically and needs deterministic, scriptable commands with structured output.

Hunk provides JSON output for every command that produces output:

```bash
$ hunk diff --json
{
  "files": [
    {
      "path": "main.go",
      "old_name": "a/main.go",
      "new_name": "b/main.go",
      "hunks": [
        {
          "old_start": 10,
          "old_lines": 5,
          "new_start": 10,
          "new_lines": 7,
          "lines": [
            {"op": "context", "content": "func Handler() {", "old_line": 10, "new_line": 10},
            {"op": "add", "content": "    if err != nil {", "new_line": 11},
            {"op": "add", "content": "        return err", "new_line": 12}
          ]
        }
      ]
    }
  ]
}
```

An agent can parse this JSON, identify which lines correspond to its changes, and construct the appropriate `hunk stage` command. No interactive prompts, no ambiguity, no manual patch construction.

The `--stage-hints` flag goes further—it tells you exactly what commands to run:

```bash
$ hunk diff --stage-hints
hunk stage main.go:11-12
hunk stage utils.go:5-8,20-25
```

## How It Works

Under the hood, hunk generates valid unified diff patches and applies them to git's staging area using `git apply --cached`. This means it's fully compatible with existing git workflows and doesn't introduce any new state or metadata.

When you run `hunk stage main.go:10-20`, hunk:

1. Runs `git diff` to get the current unstaged changes
2. Parses the diff into a structured representation with line number tracking
3. Filters the diff to include only the lines you specified (plus necessary context)
4. Generates a valid unified diff patch for just those lines
5. Applies the patch to the staging area via `git apply --cached`

The key insight is that git's patch format is well-specified and `git apply` handles all the edge cases around context lines, hunk headers, and line number calculation. Hunk just needs to generate the right patch.

## Comparison with Alternatives

**git add -p**: Interactive, designed for humans. Works at hunk granularity, not line granularity. Not easily scriptable.

**git add -N + git add -e**: Opens an editor. Not suitable for programmatic use.

**Manually constructing patches**: Error-prone. Hunk header calculation is fiddly, context lines need careful handling, and off-by-one errors are common.

**git-split-diffs and similar tools**: Focused on viewing diffs, not staging them.

Hunk fills the gap: programmatic, line-level, non-interactive staging.

## Development

The codebase follows a functional core / imperative shell architecture. The parsing and patch generation logic is pure and heavily tested. The git interaction layer is isolated behind an interface for testability.

```bash
make lint       # run linters
make unit       # run tests
make unit-cover # run tests with coverage
make check      # run both
```

See [docs/architecture.md](docs/architecture.md) for design details and [docs/testing.md](docs/testing.md) for the testing strategy.

## License

MIT
