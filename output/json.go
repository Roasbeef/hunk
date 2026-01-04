// Package output provides formatting for diff output.
package output

import (
	"encoding/json"
	"io"

	"github.com/roasbeef/hunk/diff"
)

// DiffOutput is the top-level JSON output structure.
type DiffOutput struct {
	Files []FileOutput `json:"files"`
}

// FileOutput represents a file in JSON output.
type FileOutput struct {
	Path    string       `json:"path"`
	OldPath string       `json:"old_path,omitempty"`
	Status  string       `json:"status"` // "modified", "new", "deleted", "renamed"
	Binary  bool         `json:"binary,omitempty"`
	Hunks   []HunkOutput `json:"hunks,omitempty"`
}

// HunkOutput represents a hunk in JSON output.
type HunkOutput struct {
	Header  string       `json:"header"`
	Section string       `json:"section,omitempty"`
	Hunks   []LineOutput `json:"lines"`
}

// LineOutput represents a line in JSON output.
type LineOutput struct {
	Op         string `json:"op"` // "add", "delete", "context"
	Content    string `json:"content"`
	OldLineNum int    `json:"old_line,omitempty"`
	NewLineNum int    `json:"new_line,omitempty"`
}

// FormatJSON writes the parsed diff as JSON.
func FormatJSON(w io.Writer, parsed *diff.ParsedDiff) error {
	output := DiffOutput{
		Files: make([]FileOutput, 0),
	}

	for file := range parsed.Files() {
		fo := FileOutput{
			Path:    file.Path(),
			OldPath: file.OldName,
			Status:  fileStatus(file),
			Binary:  file.IsBinary,
			Hunks:   make([]HunkOutput, 0, len(file.Hunks)),
		}

		if fo.OldPath == fo.Path {
			fo.OldPath = ""
		}

		for _, hunk := range file.Hunks {
			ho := HunkOutput{
				Header:  hunk.Header(),
				Section: hunk.Section,
				Hunks:   make([]LineOutput, 0, len(hunk.Lines)),
			}

			for _, line := range hunk.Lines {
				lo := LineOutput{
					Op:         line.Op.String(),
					Content:    line.Content,
					OldLineNum: line.OldLineNum,
					NewLineNum: line.NewLineNum,
				}
				ho.Hunks = append(ho.Hunks, lo)
			}

			fo.Hunks = append(fo.Hunks, ho)
		}

		output.Files = append(output.Files, fo)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(output)
}

// fileStatus returns the status string for a file.
func fileStatus(f *diff.FileDiff) string {
	switch {
	case f.IsNew:
		return "new"
	case f.IsDeleted:
		return "deleted"
	case f.IsRenamed:
		return "renamed"
	default:
		return "modified"
	}
}

// FormatJSONEmpty writes an empty JSON response.
func FormatJSONEmpty(w io.Writer) error {
	_, err := w.Write([]byte("{\"files\": []}\n"))

	return err
}
