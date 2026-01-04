# Architecture

This document explains how hunk is designed and why it's built the way it is.

## Design Philosophy

Hunk exists because AI agents need a different interface to git than humans do. The core insight is that agents know exactly which lines they modified—they don't need to interactively review hunks or make judgment calls about context. They need a deterministic way to say "stage these specific lines" and have it work.

The design optimizes for three properties: correctness (never stage the wrong lines), predictability (same input always produces same output), and parseability (agents can consume the output programmatically).

## Functional Core / Imperative Shell

The codebase is structured around the functional core / imperative shell pattern. This isn't just architectural aesthetics—it has practical consequences for testing and reliability.

The **functional core** lives in `diff/`, `patch/`, and `output/`. These packages contain pure functions that take data in and return data out. They don't touch the filesystem, don't shell out to git, don't have side effects. This makes them trivially testable with table-driven unit tests and amenable to property-based testing.

The **imperative shell** lives in `git/` and `commands/`. This is where side effects happen: running git commands, reading files, writing output. The shell is kept deliberately thin—it orchestrates the core functions but contains minimal logic of its own.

```
┌─────────────────────────────────────────────────────────────┐
│                      CLI (commands/)                        │
│  diff | stage | preview | commit | reset | apply-patch      │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│              Functional Core (Pure, Testable)               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ diff/parse   │  │ diff/select  │  │ patch/gen    │      │
│  │ diff/hunk    │  │ diff/file    │  │ output/json  │      │
│  │ diff/line    │  │ diff/iter    │  │ output/text  │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
└────────────────────────┬────────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────────┐
│              Imperative Shell (I/O, Side Effects)           │
│  ┌────────────────────────────────────────────────────┐    │
│  │ git.Executor interface → ShellExecutor (real git)  │    │
│  └────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

The benefit of this split becomes obvious when you try to test the system. The core logic can be tested with synthetic diffs—you don't need a real git repository to verify that line selection or patch generation works correctly. The shell logic is tested with integration tests that create real (temporary) git repositories, but those tests are focused on "did we wire everything up correctly" rather than "is the algorithm right."

## Package Overview

### diff/

This package handles parsing unified diffs and selecting lines from them.

The central types are `DiffLine`, `Hunk`, and `FileDiff`. A diff is parsed into a tree structure: a `ParsedDiff` contains `FileDiff`s, which contain `Hunk`s, which contain `DiffLine`s. Each line tracks whether it's context, addition, or deletion, along with its line numbers in both the old and new files.

Line number tracking is the trickiest part of the parser. Git's unified diff format gives hunk headers like `@@ -10,5 +10,7 @@`, meaning "starting at line 10 in the old file, 5 lines; starting at line 10 in the new file, 7 lines." But within the hunk, you have to count lines yourself, incrementing the old line number for context and deletion lines, and the new line number for context and addition lines.

The `select.go` file handles the `FILE:LINES` syntax. This is deliberately simple—a file path, a colon, and comma-separated line numbers or ranges. We could have made it more sophisticated (regex support, symbol-based selection, etc.) but simplicity wins here. Agents can easily construct these strings programmatically.

The package uses Go 1.23's iterator patterns (`iter.Seq`) for traversing diffs. This allows chaining operations like filtering and mapping without allocating intermediate slices for every step. The `iterator.go` file provides helpers like `FilteredLines`, `MapLines`, and `CollectLines` that compose cleanly.

### patch/

This package generates unified diff patches from selections.

Given a parsed diff and a set of file selections, `Generate()` produces a byte slice containing a valid unified diff that, when applied, stages exactly the selected lines. The tricky part is handling context lines correctly—git's apply requires context lines to match the working tree, and hunk headers must be recalculated based on which lines are actually included.

The algorithm walks through each selected file and hunk, filtering lines to include only those in the selection plus necessary context, then recalculates the hunk header based on what's included. The `buildFilteredHunk` function handles the details of maintaining proper old/new line counts and ensuring context lines are preserved.

### output/

This package formats diffs for display. There are two primary outputs: text (for humans) and JSON (for agents).

The text formatter adds line numbers, colors additions green and deletions red (using ANSI codes), and groups output by file. The JSON formatter produces structured output that's easy for agents to parse, with explicit fields for line numbers and operation types.

The formatter functions take an `io.Writer` so output can be captured in tests or redirected as needed. This is a small detail but important for testability.

### git/

This package abstracts git operations behind an interface:

```go
type Executor interface {
    Diff(ctx context.Context, paths ...string) (string, error)
    DiffCached(ctx context.Context, paths ...string) (string, error)
    ApplyPatch(ctx context.Context, patch io.Reader) error
    Commit(ctx context.Context, message string) error
    Reset(ctx context.Context) error
    ResetPath(ctx context.Context, path string) error
    Status(ctx context.Context) (*RepoStatus, error)
    Root(ctx context.Context) (string, error)
}
```

The production implementation, `ShellExecutor`, shells out to the git binary. This might seem crude compared to using libgit2 or a pure Go implementation, but it has significant advantages: git is ubiquitous, battle-tested, and handles edge cases we'd otherwise have to reimplement. The cost is subprocess overhead, which is negligible for the operations hunk performs.

The interface exists primarily for testability. Integration tests use `ShellExecutor` with real git repositories in temp directories. If we ever needed a mock implementation for specific test scenarios, the interface is there.

### commands/

This package implements the CLI using Cobra. Each command is a thin layer that parses flags, calls into the functional core, and writes output.

Commands receive configuration through context rather than global variables. The root command's `PersistentPreRun` hook stores the `--dir` and `--json` flags in the context, and subcommands retrieve them via `getConfig(ctx)`. This eliminates global state and makes commands easy to test in isolation.

Output goes through the command's writer (`cmd.OutOrStdout()`) rather than directly to `os.Stdout`. This allows tests to capture output without redirecting the process's stdout.

### testutil/

The test harness provides `GitTestRepo` for creating temporary git repositories and `ComparisonTest` for verifying hunk produces identical results to manual git operations.

`ComparisonTest` is particularly useful. It creates two identical repositories, applies operations to each using different methods (one with raw git commands, one with hunk commands), then asserts that the resulting state is identical. This catches subtle bugs where hunk's behavior diverges from git's.

## Data Flow

Here's how `hunk stage main.go:10-20` works end to end:

1. **Parse arguments**: The stage command parses `main.go:10-20` into a `FileSelection` with path `main.go` and ranges `[{10, 20}]`.

2. **Get unstaged diff**: The command calls `executor.Diff(ctx)`, which runs `git diff` and returns the raw unified diff text.

3. **Parse diff**: The diff text is parsed into a `ParsedDiff` structure with full line number tracking.

4. **Generate patch**: The `patch.Generate()` function takes the parsed diff and selections, filters to include only matching lines (plus context), and produces a new unified diff.

5. **Apply patch**: The command calls `executor.ApplyPatch(ctx, reader)`, which runs `git apply --cached` with the generated patch as stdin.

6. **Report success**: The command writes confirmation to stdout.

If anything goes wrong at any step, an error propagates up and the command exits non-zero.

## Error Handling

Errors are returned, not panicked. Functions that can fail return `(result, error)` tuples. Commands check errors at each step and return early with a descriptive message.

User-facing error messages try to be actionable. "no unstaged changes" tells you why the command couldn't proceed. "invalid selection syntax: expected FILE:LINES, got 'foo'" tells you how to fix the input.

## Extensibility

The design is intentionally simple and focused. There's no plugin system, no configuration file, no hooks mechanism. This is a tool for a specific purpose, and complexity would hurt more than help.

That said, the structure allows extension if needed:

New commands can be added by implementing a Cobra command and wiring it into the root command. New output formats can be added by implementing a formatter function in `output/`. The git interface could be swapped for a mock or a different implementation if needed.

## Performance Considerations

Hunk shells out to git for each operation. For the typical workflow (one diff, one stage, one commit), this means maybe three subprocess invocations. The overhead is negligible.

The parsed diff representation is held in memory. For extremely large diffs (thousands of files, millions of lines), this could be a problem. In practice, AI agents work on manageable change sets, and we've not seen memory issues.

Iterator patterns avoid allocating intermediate slices when traversing diffs, which helps for larger diffs but isn't critical for typical use.

## Concurrency

Hunk doesn't use concurrency internally. Commands run sequentially, processing completes before returning. This is simpler and sufficient for the use case.

Multiple hunk processes can run concurrently against the same repository, though they'll contend on git's index lock just like any other git operations would.

## Future Directions

Potential improvements that would fit the current architecture:

Hunk splitting (staging part of a git hunk) is partially implemented in the patch generation logic but not fully exposed. The infrastructure is there if needed.

Symbol-based selection ("stage the changes to function `processRequest`") would require parsing the source language to identify symbol boundaries. This is more complex but could be layered on top of the current line-based selection.

Watch mode (automatically staging changes as files are saved) would be a new command that wraps the existing stage logic with filesystem watching.

None of these require architectural changes—they'd be additional features built on the existing foundation.
