# Testing Guide

This document describes the testing strategy and test harness for hunk.

## Architecture

Hunk follows a **functional core / imperative shell** architecture:

- **Functional Core** (`diff/`, `patch/`, `output/`): Pure functions with no side effects. Easy to test with unit tests.
- **Imperative Shell** (`git/`, `commands/`): Handles I/O and side effects. Tested via integration tests.

## Test Categories

### Unit Tests

Located alongside source files (`*_test.go`). Test pure functions with table-driven tests.

```bash
# Run all unit tests
make unit

# Run with verbose output
make unit-verbose

# Run specific package
go test -v ./diff/...
```

### Integration Tests

Use the test harness (`testutil/`) to create real git repositories for testing.

```go
func TestStaging(t *testing.T) {
    repo := testutil.NewGitTestRepo(t)

    repo.WriteFile("main.go", "package main\n\nfunc main() {}\n")
    repo.CommitAll("initial")

    repo.WriteFile("main.go", "package main\n\n// Added.\nfunc main() {}\n")

    // Test hunk operations here...
}
```

### Comparison Tests

Verify hunk produces identical results to manual git operations.

```go
func TestHunkVsGit(t *testing.T) {
    setup := func(r *testutil.GitTestRepo) {
        r.WriteFile("main.go", "package main\n\nfunc main() {}\n")
        r.CommitAll("initial")
        r.WriteFile("main.go", "package main\n\n// Changed.\nfunc main() {}\n")
    }

    ct := testutil.NewComparisonTest(t, setup)

    // Expected: use git apply --cached
    ct.Expected.Git("apply", "--cached", patchFile)

    // Actual: use hunk stage
    // runStageCommand(ct.Actual.Dir, "main.go:3")

    // Verify identical results
    ct.AssertSameDiff()
}
```

## Test Harness

### GitTestRepo

Creates a temporary git repository with helper methods:

```go
type GitTestRepo struct {
    Dir string  // Temporary directory path
}

// Create a new test repo
repo := testutil.NewGitTestRepo(t)

// File operations
repo.WriteFile("path", "content")
repo.ReadFile("path") string
repo.FileExists("path") bool

// Git operations
repo.Git("add", "-A")
repo.CommitAll("message")
repo.StageFile("path")
repo.Diff() string
repo.DiffCached() string
```

### ComparisonTest

Manages two identical repos for comparison testing:

```go
type ComparisonTest struct {
    Expected *GitTestRepo  // For manual git operations
    Actual   *GitTestRepo  // For hunk operations
}

// Create with identical setup
ct := testutil.NewComparisonTest(t, setupFunc)

// Assertions
ct.AssertSameContent("file.go")      // Same file content
ct.AssertSameDiff()                   // Same staged diff
ct.AssertSameUnstagedDiff()           // Same unstaged diff
```

## Claude-Driven Testing Pattern

For AI agent testing with two directories in `/tmp`:

```bash
#!/bin/bash
# Setup test directories
TEST_DIR=$(mktemp -d)
EXPECTED="$TEST_DIR/expected"
ACTUAL="$TEST_DIR/actual"

# Initialize identical repos
for dir in "$EXPECTED" "$ACTUAL"; do
    mkdir -p "$dir"
    cd "$dir"
    git init
    git config user.email "test@test.com"
    git config user.name "Test"

    # Create initial state
    echo 'package main

func main() {
    println("hello")
}

func helper() {
    return
}' > main.go

    git add -A && git commit -m "initial"

    # Make changes
    echo 'package main

// Added comment 1.
func main() {
    println("hello, world")
}

// Added comment 2.
func helper() {
    return nil
}' > main.go
done

# Expected: Use git to stage lines 3-5
cd "$EXPECTED"
# ... manual git operations

# Actual: Use hunk to stage lines 3-5
cd "$ACTUAL"
hunk stage main.go:3-5

# Compare results
diff <(cd "$EXPECTED" && git diff --cached) \
     <(cd "$ACTUAL" && git diff --cached)
```

## Coverage

Target: **80%+ coverage** on functional core packages.

```bash
# Run tests with coverage
make unit-cover

# View coverage report
go tool cover -html=coverage.txt
```

### Coverage by Package

| Package | Target | Notes |
|---------|--------|-------|
| `diff/` | 90%+ | Core parsing logic |
| `patch/` | 85%+ | Patch generation |
| `output/` | 70%+ | Output formatting |
| `git/` | Integration only | Uses real git |
| `commands/` | Integration only | Uses real git |

## Property-Based Testing

Use [rapid](https://pgregory.net/rapid) for property-based tests:

```go
func TestLineRangeProperties(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        start := rapid.IntRange(1, 1000).Draw(t, "start")
        end := rapid.IntRange(start, start+100).Draw(t, "end")

        r := diff.LineRange{Start: start, End: end}

        // Property: Contains should return true for all lines in range
        for i := start; i <= end; i++ {
            if !r.Contains(i) {
                t.Fatalf("range %v should contain %d", r, i)
            }
        }

        // Property: Contains should return false for lines outside range
        if r.Contains(start - 1) {
            t.Fatalf("range %v should not contain %d", r, start-1)
        }
    })
}
```

## Test Fixtures

Test diffs are stored in `testdata/diffs/`:

```
testdata/
└── diffs/
    ├── simple_add.diff
    ├── simple_delete.diff
    ├── multiple_hunks.diff
    ├── multiple_files.diff
    ├── new_file.diff
    ├── deleted_file.diff
    └── binary_file.diff
```

## Running Tests

```bash
# All tests
make unit

# With coverage
make unit-cover

# With race detector
make unit-race

# Verbose output
make unit-verbose

# Specific package
go test -v ./diff/...

# Specific test
go test -v ./diff/... -run TestParse

# All checks (lint + test)
make check
```

## Writing New Tests

1. **Unit tests**: Add to existing `*_test.go` file or create new one
2. **Integration tests**: Use `testutil.NewGitTestRepo(t)`
3. **Comparison tests**: Use `testutil.NewComparisonTest(t, setup)`
4. **Property tests**: Add rapid-based tests for invariants

### Test Naming Convention

```go
func TestTypeName(t *testing.T) { }           // Type-level tests
func TestTypeName_MethodName(t *testing.T) { } // Method-level tests
func TestFunctionName(t *testing.T) { }       // Function-level tests
```

### Table-Driven Tests

```go
func TestParseSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    Result
        wantErr bool
    }{
        {
            name:  "valid input",
            input: "...",
            want:  Result{...},
        },
        {
            name:    "invalid input",
            input:   "...",
            wantErr: true,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := ParseSomething(tc.input)
            if tc.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            require.Equal(t, tc.want, got)
        })
    }
}
```
