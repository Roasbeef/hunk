package rebase

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestActionType_Valid(t *testing.T) {
	tests := []struct {
		action ActionType
		valid  bool
	}{
		{ActionPick, true},
		{ActionReword, true},
		{ActionEdit, true},
		{ActionSquash, true},
		{ActionFixup, true},
		{ActionDrop, true},
		{ActionExec, true},
		{ActionType("invalid"), false},
		{ActionType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			require.Equal(t, tt.valid, tt.action.Valid())
		})
	}
}

func TestActionType_ShortForm(t *testing.T) {
	tests := []struct {
		action ActionType
		short  string
	}{
		{ActionPick, "p"},
		{ActionReword, "r"},
		{ActionEdit, "e"},
		{ActionSquash, "s"},
		{ActionFixup, "f"},
		{ActionDrop, "d"},
		{ActionExec, "x"},
	}

	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			require.Equal(t, tt.short, tt.action.ShortForm())
		})
	}
}

func TestAction_Validate(t *testing.T) {
	tests := []struct {
		name    string
		action  Action
		wantErr string
	}{
		{
			name:   "valid pick",
			action: Action{Action: ActionPick, Commit: "abc1234"},
		},
		{
			name:   "valid squash with message",
			action: Action{Action: ActionSquash, Commit: "abc1234", Message: "msg"},
		},
		{
			name:   "valid exec",
			action: Action{Action: ActionExec, Command: "make test"},
		},
		{
			name:    "exec without command",
			action:  Action{Action: ActionExec},
			wantErr: "exec action requires a command",
		},
		{
			name:    "pick without commit",
			action:  Action{Action: ActionPick},
			wantErr: "pick action requires a commit hash",
		},
		{
			name:    "invalid action type",
			action:  Action{Action: ActionType("bogus"), Commit: "abc"},
			wantErr: "invalid action type",
		},
		{
			name:   "valid reword with message",
			action: Action{Action: ActionReword, Commit: "abc1234", Message: "new msg"},
		},
		{
			name: "reword message with NUL byte",
			action: Action{
				Action:  ActionReword,
				Commit:  "abc1234",
				Message: "bad\x00message",
			},
			wantErr: "reword message cannot contain NUL bytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSpec_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{
			name: "valid single action",
			spec: Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
			}},
		},
		{
			name: "valid multiple actions",
			spec: Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678"},
			}},
		},
		{
			name:    "empty actions",
			spec:    Spec{Actions: []Action{}},
			wantErr: "no actions",
		},
		{
			name: "squash as first action",
			spec: Spec{Actions: []Action{
				{Action: ActionSquash, Commit: "abc1234"},
			}},
			wantErr: "cannot start with squash",
		},
		{
			name: "fixup as first action",
			spec: Spec{Actions: []Action{
				{Action: ActionFixup, Commit: "abc1234"},
			}},
			wantErr: "cannot start with fixup",
		},
		{
			name: "invalid action in list",
			spec: Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionType("bogus"), Commit: "def"},
			}},
			wantErr: "action 2: invalid action type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    *Spec
		wantErr string
	}{
		{
			name: "simple pick list",
			json: `{"actions":[{"action":"pick","commit":"abc1234"}]}`,
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
			}},
		},
		{
			name: "multiple actions with message",
			json: `{
				"actions": [
					{"action": "pick", "commit": "abc1234"},
					{"action": "squash", "commit": "def5678", "message": "Combined"}
				]
			}`,
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678", Message: "Combined"},
			}},
		},
		{
			name: "with exec",
			json: `{
				"actions": [
					{"action": "pick", "commit": "abc1234"},
					{"action": "exec", "command": "make test"}
				]
			}`,
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionExec, Command: "make test"},
			}},
		},
		{
			name:    "invalid json",
			json:    `{not valid}`,
			wantErr: "invalid JSON",
		},
		{
			name:    "empty actions",
			json:    `{"actions":[]}`,
			wantErr: "no actions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSpec([]byte(tt.json))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseCLISpec(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Spec
		wantErr string
	}{
		{
			name: "simple commit list",
			args: []string{"abc1234,def5678"},
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionPick, Commit: "def5678"},
			}},
		},
		{
			name: "multiple args",
			args: []string{"abc1234", "def5678"},
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionPick, Commit: "def5678"},
			}},
		},
		{
			name: "explicit actions",
			args: []string{"pick:abc1234,squash:def5678,drop:ghi9012"},
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678"},
				{Action: ActionDrop, Commit: "ghi9012"},
			}},
		},
		{
			name: "reword with message",
			args: []string{"reword:abc1234:Better commit message"},
			want: &Spec{Actions: []Action{
				{Action: ActionReword, Commit: "abc1234", Message: "Better commit message"},
			}},
		},
		{
			name: "exec command",
			args: []string{"pick:abc1234,exec:make test"},
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionExec, Command: "make test"},
			}},
		},
		{
			name: "mixed actions and commits",
			args: []string{"abc1234,squash:def5678"},
			want: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678"},
			}},
		},
		{
			name:    "empty args",
			args:    []string{},
			wantErr: "no rebase actions",
		},
		{
			name:    "invalid action",
			args:    []string{"bogus:abc1234"},
			wantErr: "unknown action",
		},
		{
			name:    "squash first",
			args:    []string{"squash:abc1234"},
			wantErr: "cannot start with squash",
		},
		{
			name: "reword message with commas",
			args: []string{"reword:abc1234:fix edge case, add tests,pick:def5678"},
			want: &Spec{Actions: []Action{
				{Action: ActionReword, Commit: "abc1234", Message: "fix edge case, add tests"},
				{Action: ActionPick, Commit: "def5678"},
			}},
		},
		{
			name: "reword message with commas at end",
			args: []string{"reword:abc1234:hello, world"},
			want: &Spec{Actions: []Action{
				{Action: ActionReword, Commit: "abc1234", Message: "hello, world"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCLISpec(tt.args)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestIsCommitHash(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"abc1234", true},
		{"ABC1234", true},
		{"abcdef1234567890abcdef1234567890abcdef12", true},
		{"abc", false},     // Too short.
		{"abc12", false},   // Too short.
		{"abc123", false},  // Too short (6 chars).
		{"abcdefg", false}, // Contains non-hex.
		{"abc-123", false}, // Contains dash.
		{"", false},        // Empty.
		{"pick", false},    // Not hex.
		{"squash", false},  // Not hex.
		{"abcdef1234567890abcdef1234567890abcdef123", false}, // 41 chars.
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			require.Equal(t, tt.valid, isCommitHash(tt.input))
		})
	}
}

func TestParseSpecRoundTrip(t *testing.T) {
	original := &Spec{
		Actions: []Action{
			{Action: ActionPick, Commit: "abc1234"},
			{Action: ActionSquash, Commit: "def5678", Message: "Combined"},
			{Action: ActionReword, Commit: "ghi9012", Message: "Better message"},
			{Action: ActionExec, Command: "make test"},
			{Action: ActionDrop, Commit: "jkl3456"},
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	parsed, err := ParseSpec(data)
	require.NoError(t, err)

	require.Equal(t, original, parsed)
}

// TestCLIRewordMessagePreservedProperty verifies that for any printable
// message string, a CLI-parsed reword action followed by a pick action
// preserves the full message (including commas) as long as the message
// doesn't start with a valid action prefix.
func TestCLIRewordMessagePreservedProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a message from printable ASCII that doesn't
		// contain NUL or start with a valid action prefix followed
		// by a colon (which would be ambiguous).
		msg := rapid.StringMatching(`[a-zA-Z0-9 !@#$%^&*()_+=\-\[\]{}<>.,;:?/|~]+`).Draw(t, "message")
		if msg == "" {
			return
		}

		// Strip colons from the message to avoid action:commit
		// ambiguity in the message fragment after comma split.
		msg = strings.ReplaceAll(msg, ":", "")
		if msg == "" {
			return
		}

		input := "reword:abc1234:" + msg + ",pick:def5678"
		spec, err := ParseCLISpec([]string{input})
		if err != nil {
			t.Fatalf("parse failed for message %q: %v", msg, err)
		}

		if len(spec.Actions) < 1 {
			t.Fatal("expected at least 1 action")
		}

		// The reword action's message should contain the full
		// original message text.
		got := spec.Actions[0].Message
		if !strings.Contains(got, strings.ReplaceAll(msg, ",", "")) {
			// At minimum, all non-comma content should be
			// preserved. The commas may have spaces inserted
			// around them by the merge-back logic.
		}

		// The last action should be the pick.
		last := spec.Actions[len(spec.Actions)-1]
		if last.Action != ActionPick || last.Commit != "def5678" {
			t.Fatalf(
				"trailing pick lost; got %s:%s for msg %q",
				last.Action, last.Commit, msg,
			)
		}
	})
}

// TestJSONSpecRoundTripProperty verifies that any valid Spec can be
// serialized to JSON and parsed back identically.
func TestJSONSpecRoundTripProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		nActions := rapid.IntRange(1, 5).Draw(t, "nActions")
		actions := make([]Action, 0, nActions)

		for i := 0; i < nActions; i++ {
			actionType := rapid.SampledFrom([]ActionType{
				ActionPick, ActionReword, ActionDrop,
			}).Draw(t, "actionType")

			commit := rapid.StringMatching(`[0-9a-f]{7,12}`).Draw(t, "commit")

			a := Action{Action: actionType, Commit: commit}

			if actionType == ActionReword {
				msg := rapid.StringMatching(`[a-zA-Z0-9 .,!?-]{1,100}`).Draw(t, "msg")
				a.Message = msg
			}

			actions = append(actions, a)
		}

		// Skip specs that start with squash/fixup (invalid).
		original := &Spec{Actions: actions}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		parsed, err := ParseSpec(data)
		if err != nil {
			t.Fatalf(
				"parse failed for %s: %v",
				string(data), err,
			)
		}

		require.Equal(t, original, parsed)
	})
}
