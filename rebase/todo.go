package rebase

import (
	"bufio"
	"fmt"
	"strings"
)

// TodoEntry represents a single entry in a git rebase todo file.
type TodoEntry struct {
	// Action is the rebase action (pick, squash, etc.).
	Action ActionType

	// Commit is the commit hash.
	Commit string

	// Subject is the commit subject line.
	Subject string
}

// ParseTodoFile parses a git rebase todo file into entries.
// Ignores comments (lines starting with #) and empty lines.
func ParseTodoFile(content string) []TodoEntry {
	var entries []TodoEntry

	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, ok := parseTodoLine(line)
		if ok {
			entries = append(entries, entry)
		}
	}

	return entries
}

// parseTodoLine parses a single todo line like "pick abc1234 commit message".
func parseTodoLine(line string) (TodoEntry, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return TodoEntry{}, false
	}

	actionStr := strings.ToLower(fields[0])
	action := expandShortAction(actionStr)

	if !action.Valid() {
		return TodoEntry{}, false
	}

	commit := fields[1]

	// Everything after the commit is the subject.
	subject := ""
	if len(fields) > 2 {
		subject = strings.Join(fields[2:], " ")
	}

	return TodoEntry{
		Action:  action,
		Commit:  commit,
		Subject: subject,
	}, true
}

// expandShortAction expands single-letter action abbreviations.
func expandShortAction(s string) ActionType {
	switch s {
	case "p", "pick":
		return ActionPick
	case "r", "reword":
		return ActionReword
	case "e", "edit":
		return ActionEdit
	case "s", "squash":
		return ActionSquash
	case "f", "fixup":
		return ActionFixup
	case "d", "drop":
		return ActionDrop
	case "x", "exec":
		return ActionExec
	default:
		return ActionType(s)
	}
}

// ToTodoFile generates a git rebase todo file from the spec.
// The output format matches what git expects.
func (s *RebaseSpec) ToTodoFile() string {
	var sb strings.Builder

	for _, action := range s.Actions {
		if action.Action == ActionExec {
			fmt.Fprintf(&sb, "exec %s\n", action.Command)

			continue
		}

		fmt.Fprintf(&sb, "%s %s\n", action.Action, action.Commit)
	}

	return sb.String()
}

// ToTodoFileWithMessages generates a todo file with message handling.
// For reword actions with messages, it uses fixup -C to set the message.
// This is a more advanced format for message control.
func (s *RebaseSpec) ToTodoFileWithMessages() string {
	var sb strings.Builder

	for _, action := range s.Actions {
		if action.Action == ActionExec {
			fmt.Fprintf(&sb, "exec %s\n", action.Command)

			continue
		}

		// Standard output for actions without custom messages.
		fmt.Fprintf(&sb, "%s %s\n", action.Action, action.Commit)
	}

	return sb.String()
}

// ValidateAgainstCommits checks that the spec actions reference valid commits
// from the original todo file.
func (s *RebaseSpec) ValidateAgainstCommits(original []TodoEntry) error {
	// Build a set of valid commits from the original.
	validCommits := make(map[string]bool)

	for _, entry := range original {
		validCommits[entry.Commit] = true

		// Also allow short prefixes.
		if len(entry.Commit) >= 7 {
			validCommits[entry.Commit[:7]] = true
		}
	}

	// Check each action.
	for i, action := range s.Actions {
		if action.Action == ActionExec {
			continue
		}

		// Check if commit matches any valid commit.
		found := false

		for validCommit := range validCommits {
			if strings.HasPrefix(validCommit, action.Commit) ||
				strings.HasPrefix(action.Commit, validCommit) {
				found = true

				break
			}
		}

		if !found {
			return fmt.Errorf(
				"action %d: commit %q not found in rebase range",
				i+1, action.Commit,
			)
		}
	}

	return nil
}

// ReorderToMatchSpec reorders the original todo entries to match the spec.
// This preserves full commit hashes and subjects from the original.
func ReorderToMatchSpec(spec *RebaseSpec, original []TodoEntry) ([]TodoEntry, error) {
	// Build a map of commits to original entries.
	commitMap := make(map[string]TodoEntry)

	for _, entry := range original {
		commitMap[entry.Commit] = entry

		// Also index by short hash.
		if len(entry.Commit) >= 7 {
			commitMap[entry.Commit[:7]] = entry
		}
	}

	var result []TodoEntry

	for _, action := range spec.Actions {
		if action.Action == ActionExec {
			result = append(result, TodoEntry{
				Action:  ActionExec,
				Subject: action.Command,
			})

			continue
		}

		// Find the original entry.
		entry, ok := findCommit(commitMap, action.Commit)
		if !ok {
			return nil, fmt.Errorf("commit %q not found", action.Commit)
		}

		// Use the spec's action but original's commit and subject.
		result = append(result, TodoEntry{
			Action:  action.Action,
			Commit:  entry.Commit,
			Subject: entry.Subject,
		})
	}

	return result, nil
}

// findCommit looks up a commit in the map, allowing prefix matching.
func findCommit(m map[string]TodoEntry, commit string) (TodoEntry, bool) {
	// Try exact match first.
	if entry, ok := m[commit]; ok {
		return entry, true
	}

	// Try prefix matching.
	for key, entry := range m {
		if strings.HasPrefix(key, commit) || strings.HasPrefix(commit, key) {
			return entry, true
		}
	}

	return TodoEntry{}, false
}

// GenerateTodoFromEntries generates a todo file from entries.
func GenerateTodoFromEntries(entries []TodoEntry) string {
	var sb strings.Builder

	for _, entry := range entries {
		if entry.Action == ActionExec {
			fmt.Fprintf(&sb, "exec %s\n", entry.Subject)

			continue
		}

		fmt.Fprintf(&sb, "%s %s %s\n", entry.Action, entry.Commit, entry.Subject)
	}

	return sb.String()
}
