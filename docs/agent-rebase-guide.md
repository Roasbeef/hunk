# Agent Rebase Guide

This guide explains how AI agents can use hunk for programmatic interactive rebases. It covers the complete workflow from listing commits to handling conflicts, with emphasis on the declarative specification format and JSON output.

## Why Programmatic Rebase?

Git's interactive rebase (`git rebase -i`) was designed for humans. It opens an editor, expects manual line reordering, and relies on visual inspection. This doesn't work for agents—they can't interact with editors or prompts.

Hunk's `rebase` command group provides a fully programmatic interface. Agents specify exactly what should happen to each commit using a declarative syntax, and hunk executes the rebase without any interactive prompts.

**The core insight**: an agent that just squashed three commits knows exactly which commits to squash. It shouldn't have to simulate keyboard input—it should be able to declare "squash these commits" and have it work.

## Core Workflow

The programmatic rebase workflow follows this pattern:

```
list → run → status → continue/abort
```

1. **list**: Get the commits that would be rebased onto the target.
2. **run**: Execute the rebase with a declarative specification.
3. **status**: Check if the rebase completed or needs intervention.
4. **continue/abort**: If conflicts occurred, resolve and continue or abort.

### Step-by-Step Example

```bash
# 1. See what commits would be rebased
hunk rebase list --onto main --json

# 2. Execute the rebase with specific actions
hunk rebase run --onto main "pick:abc123,squash:def456,squash:ghi789"

# 3. If conflicts occur, check status
hunk rebase status --json

# 4. After resolving conflicts, continue
git add resolved_file.go
hunk rebase continue

# Or abort if things went wrong
hunk rebase abort
```

## Listing Commits

Before rebasing, use `list` to see which commits would be included.

```bash
hunk rebase list --onto main --json
```

### JSON Output Schema

```json
{
  "base": "main",
  "head": "HEAD",
  "commits": [
    {
      "hash": "abc123def456789...",
      "short_hash": "abc123d",
      "subject": "Add feature X",
      "author": "Agent <agent@example.com>",
      "date": "2024-01-15T10:30:00Z",
      "position": 1
    },
    {
      "hash": "def456789abc123...",
      "short_hash": "def4567",
      "subject": "Fix bug in feature X",
      "author": "Agent <agent@example.com>",
      "date": "2024-01-15T11:00:00Z",
      "position": 2
    }
  ],
  "count": 2
}
```

**Field Reference**:

| Field | Type | Description |
|-------|------|-------------|
| `base` | string | The reference being rebased onto |
| `head` | string | Always "HEAD" |
| `commits` | array | Commits to be rebased (oldest first) |
| `commits[].hash` | string | Full commit hash |
| `commits[].short_hash` | string | Abbreviated hash (7 characters) |
| `commits[].subject` | string | First line of commit message |
| `commits[].author` | string | Author name and email |
| `commits[].date` | string | ISO 8601 timestamp |
| `commits[].position` | integer | 1-based position in the rebase |
| `count` | integer | Total number of commits |

**Note**: Commits are ordered from oldest to newest (same order as `git log --reverse`). The first commit is the one that will be applied first during rebase.

## CLI Action Syntax

Actions can be specified directly on the command line using a comma-separated format.

### Basic Syntax

```
ACTION:COMMIT
```

Where:
- `ACTION` is one of: `pick`, `reword`, `edit`, `squash`, `fixup`, `drop`, `exec`
- `COMMIT` is a commit hash (short or full)

If `ACTION:` is omitted, `pick` is assumed.

### Action Reference

| Action | Syntax | Description |
|--------|--------|-------------|
| `pick` | `pick:HASH` or just `HASH` | Use commit as-is |
| `reword` | `reword:HASH:MESSAGE` | Use commit with new message |
| `edit` | `edit:HASH` | Stop for amending |
| `squash` | `squash:HASH` | Combine with previous, concat messages |
| `fixup` | `fixup:HASH` | Combine with previous, discard message |
| `drop` | `drop:HASH` | Remove commit from history |
| `exec` | `exec:COMMAND` | Run shell command |

### Examples

```bash
# Pick all commits (default action)
hunk rebase run --onto main abc123,def456,ghi789

# Explicit picks
hunk rebase run --onto main pick:abc123,pick:def456,pick:ghi789

# Squash two commits into the first
hunk rebase run --onto main pick:abc123,squash:def456

# Drop a commit
hunk rebase run --onto main pick:abc123,drop:def456,pick:ghi789

# Reword a commit
hunk rebase run --onto main "reword:abc123:Better commit message",pick:def456

# Run tests after each commit
hunk rebase run --onto main pick:abc123,exec:make\ test,pick:def456

# Complex sequence
hunk rebase run --onto main \
  pick:abc123,\
  squash:def456,\
  "reword:ghi789:Combined feature implementation",\
  exec:make\ lint
```

### Quoting Messages

Messages containing spaces must be quoted at the shell level:

```bash
# Double quotes for the whole argument
hunk rebase run --onto main "reword:abc123:Fix authentication bug"

# Or escape spaces
hunk rebase run --onto main reword:abc123:Fix\ authentication\ bug
```

## JSON Spec Format

For complex rebases, use a JSON specification file with `--spec`.

### Schema

```json
{
  "actions": [
    {
      "action": "pick",
      "commit": "abc123def456789"
    },
    {
      "action": "squash",
      "commit": "def456789abc123",
      "message": "Combined commit message"
    },
    {
      "action": "reword",
      "commit": "ghi789abc123def",
      "message": "New commit message for this commit"
    },
    {
      "action": "exec",
      "command": "make test"
    }
  ]
}
```

**Field Reference**:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `actions` | array | Yes | Ordered list of rebase operations |
| `actions[].action` | string | Yes | One of: `pick`, `reword`, `edit`, `squash`, `fixup`, `drop`, `exec` |
| `actions[].commit` | string | For non-exec | Commit hash to operate on |
| `actions[].message` | string | No | New message for `reword` or `squash` |
| `actions[].command` | string | For exec | Shell command to run |

### Using JSON Specs

```bash
# From a file
hunk rebase run --onto main --spec rebase-plan.json

# From stdin (use - for stdin)
echo '{"actions":[{"action":"pick","commit":"abc123"}]}' | \
  hunk rebase run --onto main --spec -
```

### Example: Squash Multiple Commits

```json
{
  "actions": [
    {
      "action": "pick",
      "commit": "abc123"
    },
    {
      "action": "squash",
      "commit": "def456"
    },
    {
      "action": "squash",
      "commit": "ghi789",
      "message": "feat: implement user authentication\n\nThis combines the initial implementation with two follow-up fixes."
    }
  ]
}
```

### Example: Clean Up Feature Branch

```json
{
  "actions": [
    {
      "action": "pick",
      "commit": "abc123"
    },
    {
      "action": "fixup",
      "commit": "def456"
    },
    {
      "action": "drop",
      "commit": "ghi789"
    },
    {
      "action": "reword",
      "commit": "jkl012",
      "message": "feat: add comprehensive error handling"
    },
    {
      "action": "exec",
      "command": "go test ./..."
    }
  ]
}
```

## Output Formats

### hunk rebase run --json

Returns the result of the rebase operation.

**Success**:

```json
{
  "success": true,
  "message": "Rebase completed successfully",
  "in_progress": false
}
```

**Conflict**:

```json
{
  "success": false,
  "message": "Rebase paused due to conflicts",
  "in_progress": true,
  "has_conflict": true
}
```

**Field Reference**:

| Field | Type | Description |
|-------|------|-------------|
| `success` | boolean | True if rebase completed without intervention needed |
| `message` | string | Human-readable status message |
| `in_progress` | boolean | True if rebase is paused (conflict or edit) |
| `has_conflict` | boolean | True if paused due to conflicts |

### hunk rebase status --json

Returns detailed status of an in-progress rebase.

```json
{
  "in_progress": true,
  "state": "conflict",
  "current_action": "pick",
  "total_commits": 5,
  "remaining_commits": 3,
  "completed_commits": 2,
  "conflicts": [
    {
      "file": "main.go",
      "conflict_type": "content"
    },
    {
      "file": "utils.go",
      "conflict_type": "content"
    }
  ],
  "original_branch": "feature-branch",
  "onto_ref": "abc123def456789",
  "instructions": [
    "Resolve conflicts in the listed files",
    "Stage resolved files with 'git add <file>'",
    "Continue with 'hunk rebase continue'",
    "Or abort with 'hunk rebase abort'"
  ]
}
```

**No Rebase in Progress**:

```json
{
  "in_progress": false,
  "state": ""
}
```

**Field Reference**:

| Field | Type | Description |
|-------|------|-------------|
| `in_progress` | boolean | True if a rebase is active |
| `state` | string | One of: `conflict`, `edit`, empty |
| `current_action` | string | The action that paused (if applicable) |
| `total_commits` | integer | Total commits in the rebase |
| `remaining_commits` | integer | Commits left to process |
| `completed_commits` | integer | Commits already processed |
| `conflicts` | array | Files with conflicts |
| `conflicts[].file` | string | Path to conflicting file |
| `conflicts[].conflict_type` | string | Type of conflict (e.g., "content") |
| `original_branch` | string | Branch being rebased |
| `onto_ref` | string | Reference being rebased onto |
| `instructions` | array | Human-readable next steps |

### hunk rebase continue/abort/skip --json

Returns the result of the control operation.

```json
{
  "success": true,
  "message": "Rebase completed successfully.",
  "in_progress": false
}
```

Or if more work remains:

```json
{
  "success": true,
  "message": "Continued. 2 commits remaining.",
  "in_progress": true
}
```

## Conflict Handling

When a rebase encounters conflicts, it pauses and waits for resolution.

### Detecting Conflicts

After `rebase run`, check the output:

```bash
$ hunk rebase run --onto main --json ...
{
  "success": false,
  "message": "Rebase paused due to conflicts",
  "in_progress": true,
  "has_conflict": true
}
```

### Getting Conflict Details

Use `status` for detailed conflict information:

```bash
$ hunk rebase status --json
{
  "in_progress": true,
  "state": "conflict",
  "conflicts": [
    {"file": "main.go", "conflict_type": "content"}
  ],
  ...
}
```

### Resolution Workflow

1. **Read the conflicting files** to understand what needs resolution.
2. **Edit the files** to resolve conflicts (remove conflict markers).
3. **Stage resolved files** using `git add`.
4. **Continue the rebase** with `hunk rebase continue`.

```bash
# After resolving conflicts manually
git add main.go
hunk rebase continue --json
```

### Skipping a Problematic Commit

If a commit cannot be cleanly applied and you want to drop it:

```bash
hunk rebase skip --json
```

This skips the current commit and continues with the next one.

### Aborting the Rebase

To abandon the rebase and restore the original state:

```bash
hunk rebase abort --json
```

This returns the branch to its state before the rebase started.

## Complete Example Session

This example shows an agent cleaning up a feature branch before merging.

```bash
# Initial state: feature branch with 4 commits
# - abc123: "Add user model"
# - def456: "Fix typo in user model"
# - ghi789: "Add user service"
# - jkl012: "WIP debugging"

# Step 1: List commits to understand the branch
$ hunk rebase list --onto main --json
{
  "base": "main",
  "commits": [
    {"hash": "abc123...", "short_hash": "abc123", "subject": "Add user model", "position": 1},
    {"hash": "def456...", "short_hash": "def456", "subject": "Fix typo in user model", "position": 2},
    {"hash": "ghi789...", "short_hash": "ghi789", "subject": "Add user service", "position": 3},
    {"hash": "jkl012...", "short_hash": "jkl012", "subject": "WIP debugging", "position": 4}
  ],
  "count": 4
}

# Step 2: Plan the rebase
# - Keep "Add user model", squash the typo fix into it
# - Keep "Add user service"
# - Drop "WIP debugging"

# Step 3: Execute the rebase
$ hunk rebase run --onto main --json \
  pick:abc123,fixup:def456,pick:ghi789,drop:jkl012
{
  "success": true,
  "message": "Rebase completed successfully",
  "in_progress": false
}

# Result: clean branch with 2 focused commits
# - "Add user model" (includes typo fix)
# - "Add user service"
```

### Handling a Conflict

```bash
# Rebase that hits a conflict
$ hunk rebase run --onto main --json pick:abc123,squash:def456
{
  "success": false,
  "message": "Rebase paused due to conflicts",
  "in_progress": true,
  "has_conflict": true
}

# Check status for details
$ hunk rebase status --json
{
  "in_progress": true,
  "state": "conflict",
  "conflicts": [
    {"file": "user.go", "conflict_type": "content"}
  ],
  "remaining_commits": 1,
  "instructions": [...]
}

# Agent resolves the conflict by editing user.go
# (removes conflict markers, keeps correct code)

# Stage the resolution
$ git add user.go

# Continue the rebase
$ hunk rebase continue --json
{
  "success": true,
  "message": "Rebase completed successfully.",
  "in_progress": false
}
```

## Security Notes

### Exec Command Validation

The `exec` action runs arbitrary shell commands. Hunk applies one security restriction: **exec commands cannot contain newlines**.

This prevents injection attacks where a malicious command could add extra lines to the rebase todo file, potentially executing unintended actions.

```json
{
  "action": "exec",
  "command": "make test\npick abc123"
}
```

This will be rejected with: `exec command cannot contain newlines`.

### Input Validation

All commit hashes are validated to ensure they:
- Are 7-40 characters long.
- Contain only hexadecimal characters (0-9, a-f, A-F).

Invalid hashes are rejected before the rebase starts.

### Action Ordering

Hunk validates that `squash` and `fixup` actions are not first in the sequence (they require a previous commit to combine with).

```json
{
  "actions": [
    {"action": "squash", "commit": "abc123"}
  ]
}
```

This will be rejected with: `cannot start with squash: no previous commit to combine with`.

## Best Practices

### Always Use JSON Mode

For reliable parsing, always include `--json`:

```bash
hunk rebase list --onto main --json
hunk rebase run --onto main --json ...
hunk rebase status --json
```

### Verify Before Running

Use `list` to confirm you understand which commits will be affected:

```bash
# Check commits first
hunk rebase list --onto main --json

# Then run if it looks right
hunk rebase run --onto main ...
```

### Use Short Hashes from List Output

The `short_hash` from `list` output is guaranteed to be unambiguous within the repository:

```bash
# Get hashes from list
commits=$(hunk rebase list --onto main --json | jq -r '.commits[].short_hash')

# Use them in run
hunk rebase run --onto main "pick:$first,squash:$second"
```

### Prefer Fixup Over Squash for Cleanup

When combining commits where you don't need the intermediate messages, use `fixup` instead of `squash`. It's cleaner and doesn't require message editing.

### Test Exec Commands Separately

Before including `exec` commands in a rebase, verify they work:

```bash
# Test the command first
make test && echo "OK"

# Then include in rebase
hunk rebase run --onto main pick:abc123,exec:make\ test,pick:def456
```

### Handle Conflicts Programmatically

When conflicts occur, parse the status JSON to understand what needs resolution:

```python
status = json.loads(subprocess.check_output(["hunk", "rebase", "status", "--json"]))
if status["in_progress"] and status["state"] == "conflict":
    for conflict in status["conflicts"]:
        resolve_conflict(conflict["file"])
    subprocess.run(["git", "add", "-A"])
    subprocess.run(["hunk", "rebase", "continue"])
```

### Abort on Unexpected Errors

If something goes wrong during rebase that you can't recover from, abort immediately to restore the original state:

```bash
hunk rebase abort
```

This is always safe—it returns the branch to its pre-rebase state.

## Integration Tips

### Working Directory

Use `--dir` to operate on a repository in a different directory:

```bash
hunk --dir /path/to/repo rebase list --onto main --json
```

### Exit Codes

All hunk commands return non-zero exit codes on errors:

- `0`: Success.
- `1`: Error (parse error, validation failure, git error).

Check exit codes in scripts:

```bash
if ! hunk rebase run --onto main ...; then
    echo "Rebase failed"
    hunk rebase abort
    exit 1
fi
```

### Stdin for Complex Specs

For very complex rebases, generate JSON and pipe it:

```python
spec = {"actions": [...]}  # Build programmatically
proc = subprocess.Popen(
    ["hunk", "rebase", "run", "--onto", "main", "--spec", "-", "--json"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
)
stdout, _ = proc.communicate(json.dumps(spec).encode())
result = json.loads(stdout)
```

### Rebase vs Cherry-Pick

Hunk's rebase is appropriate when you want to rewrite commit history on a branch. If you just need to apply specific commits elsewhere, consider `git cherry-pick` instead.
