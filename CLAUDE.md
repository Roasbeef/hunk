# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Build and Test Commands

```bash
make build          # Build hunk binary
make install        # Install to $GOPATH/bin
make lint           # Run golangci-lint
make unit           # Run all tests
make unit-cover     # Run tests with coverage report
make unit-race      # Run tests with race detector
make check          # Run lint + unit tests
make fmt            # Format code with goimports and gofmt
go test -v ./diff/... -run TestParse  # Run specific test
```

## Architecture

Hunk follows a **functional core / imperative shell** architecture:

```
CLI (commands/) → Functional Core (diff/, patch/, output/) → Git Shell (git/)
```

**Functional Core** (pure, no side effects):
- `diff/` - Unified diff parsing with line number tracking. Uses Go 1.23 `iter.Seq` for iteration.
- `patch/` - Generates valid unified diff patches from line selections.
- `output/` - JSON and text formatters. Takes `io.Writer` for testability.

**Imperative Shell** (I/O, side effects):
- `git/` - Executor interface abstracting git operations. `ShellExecutor` shells out to git.
- `commands/` - Cobra CLI commands. Config passed via context, not globals.

## Key Patterns

**Context-based config**: Commands use `getConfig(ctx)` to retrieve `--dir` and `--json` flags stored by root command's `PersistentPreRun`.

**Testable output**: All commands write to `cmd.OutOrStdout()`, not `os.Stdout` directly.

**Test harness**: `testutil.GitTestRepo` creates temp git repos. `ComparisonTest` verifies hunk matches git behavior.

**Property-based testing**: Uses `pgregory.net/rapid` with `rapid.Check` and `rapid.MakeFuzz` for fuzz tests.

## FILE:LINES Syntax

The core staging syntax is `FILE:LINES` where LINES is comma-separated ranges:
- `main.go:10` - Single line
- `main.go:10-20` - Range
- `main.go:10-20,30-40` - Multiple ranges
- `main.go:10 utils.go:5-8` - Multiple files (space-separated args)

Line numbers refer to **new file** lines (after edits), matching how editors display content.

## Code Style

- 80-character line limit (not enforced by linter, trust developers).
- Comments use complete sentences ending with periods.
- JSON field names use snake_case (`old_line`, `new_line`).
- Comments focus on the why/how vs just the plain what.
- No `internal/` packages - all packages are at top level.
- Test files use `_test` suffix but stay in same package (not `package foo_test`).
- Test function names use camelCase, not underscores (e.g., `TestRebaseRunSimple` not `TestRebaseRun_Simple`).
