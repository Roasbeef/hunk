package rebase

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestParseTodoFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []TodoEntry
	}{
		{
			name: "simple picks",
			content: `pick abc1234 First commit
pick def5678 Second commit
`,
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
			},
		},
		{
			name: "with comments and empty lines",
			content: `pick abc1234 First commit

# This is a comment
pick def5678 Second commit

# Commands:
# p, pick = use commit
`,
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
			},
		},
		{
			name: "various actions",
			content: `pick abc1234 First commit
squash def5678 Second commit
reword ghi9012 Third commit
fixup jkl3456 Fourth commit
drop mno7890 Fifth commit
`,
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionSquash, Commit: "def5678", Subject: "Second commit"},
				{Action: ActionReword, Commit: "ghi9012", Subject: "Third commit"},
				{Action: ActionFixup, Commit: "jkl3456", Subject: "Fourth commit"},
				{Action: ActionDrop, Commit: "mno7890", Subject: "Fifth commit"},
			},
		},
		{
			name: "short actions",
			content: `p abc1234 First
s def5678 Second
r ghi9012 Third
f jkl3456 Fourth
d mno7890 Fifth
`,
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First"},
				{Action: ActionSquash, Commit: "def5678", Subject: "Second"},
				{Action: ActionReword, Commit: "ghi9012", Subject: "Third"},
				{Action: ActionFixup, Commit: "jkl3456", Subject: "Fourth"},
				{Action: ActionDrop, Commit: "mno7890", Subject: "Fifth"},
			},
		},
		{
			name:    "empty content",
			content: "",
			want:    nil,
		},
		{
			name: "only comments",
			content: `# Just a comment
# Another comment
`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTodoFile(tt.content)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestSpec_ToTodoFile(t *testing.T) {
	tests := []struct {
		name string
		spec *Spec
		want string
	}{
		{
			name: "simple picks",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionPick, Commit: "def5678"},
			}},
			want: "pick abc1234\npick def5678\n",
		},
		{
			name: "various actions",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678"},
				{Action: ActionDrop, Commit: "ghi9012"},
			}},
			want: "pick abc1234\nsquash def5678\ndrop ghi9012\n",
		},
		{
			name: "with exec",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionExec, Command: "make test"},
				{Action: ActionPick, Commit: "def5678"},
			}},
			want: "pick abc1234\nexec make test\npick def5678\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.spec.ToTodoFile()
			require.Equal(t, tt.want, got)
		})
	}
}

func TestValidateAgainstCommits(t *testing.T) {
	original := []TodoEntry{
		{Action: ActionPick, Commit: "abc1234def5678", Subject: "First"},
		{Action: ActionPick, Commit: "111222333444555", Subject: "Second"},
		{Action: ActionPick, Commit: "aaabbbcccdddeee", Subject: "Third"},
	}

	tests := []struct {
		name    string
		spec    *Spec
		wantErr string
	}{
		{
			name: "valid - full hashes",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234def5678"},
				{Action: ActionSquash, Commit: "111222333444555"},
			}},
		},
		{
			name: "valid - short hashes",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "1112223"},
			}},
		},
		{
			name: "valid - with exec",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionExec, Command: "make test"},
			}},
		},
		{
			name: "invalid commit",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "nonexistent"},
			}},
			wantErr: "not found in rebase range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.ValidateAgainstCommits(original)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestReorderToMatchSpec(t *testing.T) {
	original := []TodoEntry{
		{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
		{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
		{Action: ActionPick, Commit: "ghi9012", Subject: "Third commit"},
	}

	tests := []struct {
		name    string
		spec    *Spec
		want    []TodoEntry
		wantErr string
	}{
		{
			name: "reverse order",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "ghi9012"},
				{Action: ActionPick, Commit: "def5678"},
				{Action: ActionPick, Commit: "abc1234"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "ghi9012", Subject: "Third commit"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
			},
		},
		{
			name: "change actions",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionSquash, Commit: "def5678"},
				{Action: ActionDrop, Commit: "ghi9012"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionSquash, Commit: "def5678", Subject: "Second commit"},
				{Action: ActionDrop, Commit: "ghi9012", Subject: "Third commit"},
			},
		},
		{
			name: "with exec",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{Action: ActionExec, Command: "make test"},
				{Action: ActionPick, Commit: "def5678"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "make test"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
			},
		},
		{
			name: "unknown commit",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "unknown"},
			}},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReorderToMatchSpec(tt.spec, original)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestReorderToMatchSpecRewordWithMessage(t *testing.T) {
	original := []TodoEntry{
		{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
		{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
		{Action: ActionPick, Commit: "ghi9012", Subject: "Third commit"},
	}

	tests := []struct {
		name string
		spec *Spec
		want []TodoEntry
	}{
		{
			name: "reword with single-line message",
			spec: &Spec{Actions: []Action{
				{Action: ActionPick, Commit: "abc1234"},
				{
					Action:  ActionReword,
					Commit:  "def5678",
					Message: "Better message",
				},
				{Action: ActionPick, Commit: "ghi9012"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
				{Action: ActionExec, Subject: "printf 'Better message' | git commit --amend -F -"},
				{Action: ActionPick, Commit: "ghi9012", Subject: "Third commit"},
			},
		},
		{
			name: "reword with multi-line message",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "Title line\n\nBody paragraph one.\nBody paragraph two.",
				},
				{Action: ActionPick, Commit: "def5678"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: `printf 'Title line\n\nBody paragraph one.\nBody paragraph two.' | git commit --amend -F -`},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
			},
		},
		{
			name: "reword message with single quotes",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "fix: don't panic on nil",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: `printf 'fix: don'\''t panic on nil' | git commit --amend -F -`},
			},
		},
		{
			name: "reword message with percent sign",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "bump coverage to 100%",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "printf 'bump coverage to 100%%' | git commit --amend -F -"},
			},
		},
		{
			name: "reword message with backslash",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: `fix path\to\file handling`,
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: `printf 'fix path\\to\\file handling' | git commit --amend -F -`},
			},
		},
		{
			name: "reword without message falls through to plain reword",
			spec: &Spec{Actions: []Action{
				{
					Action: ActionReword,
					Commit: "abc1234",
				},
			}},
			want: []TodoEntry{
				{Action: ActionReword, Commit: "abc1234", Subject: "First commit"},
			},
		},
		{
			name: "multiple rewords with messages",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "New first",
				},
				{
					Action:  ActionReword,
					Commit:  "def5678",
					Message: "New second",
				},
				{Action: ActionPick, Commit: "ghi9012"},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "printf 'New first' | git commit --amend -F -"},
				{Action: ActionPick, Commit: "def5678", Subject: "Second commit"},
				{Action: ActionExec, Subject: "printf 'New second' | git commit --amend -F -"},
				{Action: ActionPick, Commit: "ghi9012", Subject: "Third commit"},
			},
		},
		{
			name: "reword message with double quotes",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: `fix: handle "edge case" in parser`,
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: `printf 'fix: handle "edge case" in parser' | git commit --amend -F -`},
			},
		},
		{
			name: "reword message with dollar sign and backtick",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "fix: handle $HOME and `uname` safely",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "printf 'fix: handle $HOME and `uname` safely' | git commit --amend -F -"},
			},
		},
		{
			name: "reword message with tab character",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "fix:\tindented message",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "printf 'fix:\tindented message' | git commit --amend -F -"},
			},
		},
		{
			name: "reword message with carriage return stripped",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "title\r\n\r\nbody line",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: `printf 'title\n\nbody line' | git commit --amend -F -`},
			},
		},
		{
			name: "reword message with exclamation mark",
			spec: &Spec{Actions: []Action{
				{
					Action:  ActionReword,
					Commit:  "abc1234",
					Message: "fix: this works!",
				},
			}},
			want: []TodoEntry{
				{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
				{Action: ActionExec, Subject: "printf 'fix: this works!' | git commit --amend -F -"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReorderToMatchSpec(tt.spec, original)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestRewordExecLineIsSingleLine verifies that the exec command generated for
// reword-with-message is always a single line, since git's rebase todo parser
// requires one command per line.
func TestRewordExecLineIsSingleLine(t *testing.T) {
	original := []TodoEntry{
		{Action: ActionPick, Commit: "abc1234", Subject: "First"},
	}

	spec := &Spec{Actions: []Action{
		{
			Action:  ActionReword,
			Commit:  "abc1234",
			Message: "Title\n\nMulti-line body\nwith several lines\nof text.",
		},
	}}

	entries, err := ReorderToMatchSpec(spec, original)
	require.NoError(t, err)

	// Generate the todo file and verify no exec line contains a raw newline
	// (escaped \n in the printf format string is fine, actual newlines are not).
	todo := GenerateTodoFromEntries(entries)
	for _, line := range strings.Split(todo, "\n") {
		if strings.HasPrefix(line, "exec ") {
			// The line itself should not contain any literal newlines
			// (it's already split on newlines, so if we got here it's
			// a single line). Verify it has the escaped form.
			require.Contains(t, line, `\n`)
			require.NotContains(t, line, "\n\n")
		}
	}
}

func TestGenerateTodoFromEntries(t *testing.T) {
	entries := []TodoEntry{
		{Action: ActionPick, Commit: "abc1234", Subject: "First commit"},
		{Action: ActionSquash, Commit: "def5678", Subject: "Second commit"},
		{Action: ActionExec, Subject: "make test"},
		{Action: ActionDrop, Commit: "ghi9012", Subject: "Third commit"},
	}

	want := `pick abc1234 First commit
squash def5678 Second commit
exec make test
drop ghi9012 Third commit
`

	got := GenerateTodoFromEntries(entries)
	require.Equal(t, want, got)
}

func TestExpandShortAction(t *testing.T) {
	tests := []struct {
		input string
		want  ActionType
	}{
		{"p", ActionPick},
		{"pick", ActionPick},
		{"r", ActionReword},
		{"reword", ActionReword},
		{"e", ActionEdit},
		{"edit", ActionEdit},
		{"s", ActionSquash},
		{"squash", ActionSquash},
		{"f", ActionFixup},
		{"fixup", ActionFixup},
		{"d", ActionDrop},
		{"drop", ActionDrop},
		{"x", ActionExec},
		{"exec", ActionExec},
		{"unknown", ActionType("unknown")},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := expandShortAction(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestRewordEscapingSingleLineProperty verifies that for any arbitrary
// message string, the generated exec line is always a single line in
// the todo file output.
func TestRewordEscapingSingleLineProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := rapid.String().Draw(t, "message")

		// Filter out NUL bytes since Validate() rejects them.
		msg = strings.ReplaceAll(msg, "\x00", "")
		if msg == "" {
			return
		}

		original := []TodoEntry{
			{Action: ActionPick, Commit: "abc1234", Subject: "Test"},
		}
		spec := &Spec{Actions: []Action{
			{
				Action:  ActionReword,
				Commit:  "abc1234",
				Message: msg,
			},
		}}

		entries, err := ReorderToMatchSpec(spec, original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		todo := GenerateTodoFromEntries(entries)
		for _, line := range strings.Split(todo, "\n") {
			if strings.HasPrefix(line, "exec ") {
				// No raw newlines allowed in exec lines.
				if strings.Contains(line, "\n") {
					t.Fatalf(
						"exec line contains raw newline "+
							"for message %q", msg,
					)
				}
			}
		}
	})
}

// TestRewordEscapingNoUnbalancedQuotesProperty verifies that the
// generated printf command always has balanced single quotes.
func TestRewordEscapingNoUnbalancedQuotesProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := rapid.String().Draw(t, "message")
		msg = strings.ReplaceAll(msg, "\x00", "")
		if msg == "" {
			return
		}

		original := []TodoEntry{
			{Action: ActionPick, Commit: "abc1234", Subject: "Test"},
		}
		spec := &Spec{Actions: []Action{
			{
				Action:  ActionReword,
				Commit:  "abc1234",
				Message: msg,
			},
		}}

		entries, err := ReorderToMatchSpec(spec, original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// The exec entry is the second one (after the pick).
		execEntry := entries[1]
		cmd := execEntry.Subject

		// The command should start with printf ' and end with
		// ' | git commit --amend -F -. Count unescaped single
		// quotes: each '\'' contributes to balanced quoting.
		if !strings.HasPrefix(cmd, "printf '") {
			t.Fatalf("unexpected command prefix: %s", cmd)
		}
		if !strings.HasSuffix(cmd, "' | git commit --amend -F -") {
			t.Fatalf("unexpected command suffix: %s", cmd)
		}
	})
}

// TestRewordNoCarriageReturnProperty verifies that carriage returns
// are always stripped from the escaped output.
func TestRewordNoCarriageReturnProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		msg := rapid.String().Draw(t, "message")
		msg = strings.ReplaceAll(msg, "\x00", "")
		if msg == "" {
			return
		}

		original := []TodoEntry{
			{Action: ActionPick, Commit: "abc1234", Subject: "Test"},
		}
		spec := &Spec{Actions: []Action{
			{
				Action:  ActionReword,
				Commit:  "abc1234",
				Message: msg,
			},
		}}

		entries, err := ReorderToMatchSpec(spec, original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		execEntry := entries[1]
		if strings.Contains(execEntry.Subject, "\r") {
			t.Fatalf(
				"exec line contains carriage return "+
					"for message %q", msg,
			)
		}
	})
}
