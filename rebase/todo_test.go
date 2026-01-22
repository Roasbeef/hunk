package rebase

import (
	"testing"

	"github.com/stretchr/testify/require"
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
