// Package rebase provides types and parsing for declarative rebase specifications.
// This enables AI agents to describe rebase operations without interactive prompts.
package rebase

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ActionType represents a rebase action (pick, squash, etc.).
type ActionType string

const (
	ActionPick   ActionType = "pick"
	ActionReword ActionType = "reword"
	ActionEdit   ActionType = "edit"
	ActionSquash ActionType = "squash"
	ActionFixup  ActionType = "fixup"
	ActionDrop   ActionType = "drop"
	ActionExec   ActionType = "exec"
)

// Valid returns true if the action type is recognized.
func (a ActionType) Valid() bool {
	switch a {
	case ActionPick, ActionReword, ActionEdit, ActionSquash,
		ActionFixup, ActionDrop, ActionExec:
		return true
	default:
		return false
	}
}

// ShortForm returns the single-letter abbreviation for the action.
func (a ActionType) ShortForm() string {
	switch a {
	case ActionPick:
		return "p"
	case ActionReword:
		return "r"
	case ActionEdit:
		return "e"
	case ActionSquash:
		return "s"
	case ActionFixup:
		return "f"
	case ActionDrop:
		return "d"
	case ActionExec:
		return "x"
	default:
		return string(a)
	}
}

// RebaseAction represents a single rebase operation.
type RebaseAction struct {
	// Action is the type of operation (pick, squash, drop, etc.).
	Action ActionType `json:"action"`

	// Commit is the commit hash (required for all actions except exec).
	Commit string `json:"commit,omitempty"`

	// Message is the commit message for reword/squash operations.
	// If empty during squash, git will prompt for message concatenation.
	Message string `json:"message,omitempty"`

	// Command is the shell command for exec actions.
	Command string `json:"command,omitempty"`
}

// Validate checks that the action is valid.
func (a *RebaseAction) Validate() error {
	if !a.Action.Valid() {
		return fmt.Errorf("invalid action type: %q", a.Action)
	}

	if a.Action == ActionExec {
		if a.Command == "" {
			return fmt.Errorf("exec action requires a command")
		}

		// Reject newlines in exec commands to prevent command injection.
		// A newline could inject additional rebase todo entries.
		if strings.Contains(a.Command, "\n") {
			return fmt.Errorf(
				"exec command cannot contain newlines",
			)
		}

		return nil
	}

	if a.Commit == "" {
		return fmt.Errorf("%s action requires a commit hash", a.Action)
	}

	return nil
}

// RebaseSpec is a complete rebase specification.
type RebaseSpec struct {
	// Actions is the ordered list of rebase operations.
	Actions []RebaseAction `json:"actions"`
}

// Validate checks that the spec is valid.
func (s *RebaseSpec) Validate() error {
	if len(s.Actions) == 0 {
		return fmt.Errorf("rebase spec has no actions")
	}

	for i, action := range s.Actions {
		if err := action.Validate(); err != nil {
			return fmt.Errorf("action %d: %w", i+1, err)
		}
	}

	// Check that squash/fixup are not first (they need a previous commit).
	if len(s.Actions) > 0 {
		first := s.Actions[0].Action
		if first == ActionSquash || first == ActionFixup {
			return fmt.Errorf(
				"cannot start with %s: no previous commit to combine with",
				first,
			)
		}
	}

	return nil
}

// ParseSpec parses a RebaseSpec from JSON data.
func ParseSpec(data []byte) (*RebaseSpec, error) {
	var spec RebaseSpec

	if err := json.Unmarshal(data, &spec); err != nil {
		// Include a snippet of the invalid JSON for debugging.
		snippet := string(data)
		if len(snippet) > 100 {
			snippet = snippet[:100] + "..."
		}

		return nil, fmt.Errorf(
			"invalid JSON spec: %w\ninput: %s", err, snippet,
		)
	}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return &spec, nil
}

// ParseCLISpec parses the CLI shorthand syntax.
//
// Supported formats:
//   - "abc123,def456" - pick all commits
//   - "pick:abc123,squash:def456" - explicit actions
//   - "reword:abc123:New message" - action with message
//   - "exec:make test" - exec command
func ParseCLISpec(args []string) (*RebaseSpec, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("no rebase actions specified")
	}

	var actions []RebaseAction

	// Join all args and split by comma.
	combined := strings.Join(args, ",")
	parts := splitPreservingQuotes(combined)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		action, err := parseActionSpec(part)
		if err != nil {
			return nil, fmt.Errorf("invalid action %q: %w", part, err)
		}

		actions = append(actions, action)
	}

	spec := &RebaseSpec{Actions: actions}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	return spec, nil
}

// parseActionSpec parses a single action specification.
// Formats: "abc123", "pick:abc123", "reword:abc123:Message", "exec:command".
func parseActionSpec(s string) (RebaseAction, error) {
	// Check if it's just a commit hash (no colon, or colon only at position 1
	// for Windows paths).
	if !strings.Contains(s, ":") || isCommitHash(s) {
		return RebaseAction{
			Action: ActionPick,
			Commit: s,
		}, nil
	}

	// Split by first colon to get action.
	colonIdx := strings.Index(s, ":")
	actionStr := strings.ToLower(s[:colonIdx])
	rest := s[colonIdx+1:]

	action := ActionType(actionStr)
	if !action.Valid() {
		// Maybe it's just a commit hash that contains colon-like pattern.
		// Try treating whole thing as commit.
		if isCommitHash(s) {
			return RebaseAction{
				Action: ActionPick,
				Commit: s,
			}, nil
		}

		return RebaseAction{}, fmt.Errorf("unknown action: %q", actionStr)
	}

	// Handle exec specially - rest is the command.
	if action == ActionExec {
		return RebaseAction{
			Action:  ActionExec,
			Command: rest,
		}, nil
	}

	// For other actions, check if there's a message after the commit.
	if strings.Contains(rest, ":") {
		// Format: action:commit:message.
		parts := strings.SplitN(rest, ":", 2)
		return RebaseAction{
			Action:  action,
			Commit:  strings.TrimSpace(parts[0]),
			Message: strings.TrimSpace(parts[1]),
		}, nil
	}

	return RebaseAction{
		Action: action,
		Commit: strings.TrimSpace(rest),
	}, nil
}

// isCommitHash checks if a string looks like a commit hash.
// Accepts 7-40 character hex strings.
func isCommitHash(s string) bool {
	if len(s) < 7 || len(s) > 40 {
		return false
	}

	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')) {
			return false
		}
	}

	return true
}

// splitPreservingQuotes splits by comma but preserves quoted strings.
func splitPreservingQuotes(s string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)

	for _, c := range s {
		switch {
		case (c == '"' || c == '\'') && !inQuotes:
			inQuotes = true
			quoteChar = c
			current.WriteRune(c)
		case c == quoteChar && inQuotes:
			inQuotes = false
			quoteChar = 0
			current.WriteRune(c)
		case c == ',' && !inQuotes:
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(c)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}
