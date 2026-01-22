package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/roasbeef/hunk/diff"
)

// Colors for terminal output.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
)

// TextOptions configures text output formatting.
type TextOptions struct {
	// Color enables ANSI color codes.
	Color bool

	// LineNumbers shows line numbers.
	LineNumbers bool

	// Stats shows +/- statistics.
	Stats bool
}

// DefaultTextOptions returns default text formatting options.
func DefaultTextOptions() TextOptions {
	return TextOptions{
		Color:       true,
		LineNumbers: true,
		Stats:       true,
	}
}

// FormatText writes the parsed diff as formatted text.
func FormatText(
	w io.Writer, parsed *diff.ParsedDiff, opts TextOptions,
) error {
	for file := range parsed.Files() {
		if err := formatFile(w, file, opts); err != nil {
			return err
		}
	}

	if opts.Stats {
		added, deleted := parsed.Stats()
		fmt.Fprintf(w, "\n%d insertions(+), %d deletions(-)\n", added, deleted)
	}

	return nil
}

func formatFile(w io.Writer, file *diff.FileDiff, opts TextOptions) error {
	// File header.
	header := file.Path()
	if file.IsRenamed {
		header = fmt.Sprintf("%s -> %s", file.OldName, file.NewName)
	}

	if opts.Color {
		fmt.Fprintf(w, "%s%s%s\n", colorCyan, header, colorReset)
	} else {
		fmt.Fprintln(w, header)
	}

	if file.IsBinary {
		fmt.Fprintln(w, "Binary file")

		return nil
	}

	for i, hunk := range file.Hunks {
		if i > 0 {
			fmt.Fprintln(w)
		}

		if err := formatHunk(w, hunk, opts); err != nil {
			return err
		}
	}

	return nil
}

func formatHunk(w io.Writer, hunk *diff.Hunk, opts TextOptions) error {
	// Hunk header.
	header := hunk.Header()
	if opts.Color {
		fmt.Fprintf(w, "%s%s%s\n", colorBlue, header, colorReset)
	} else {
		fmt.Fprintln(w, header)
	}

	for _, line := range hunk.Lines {
		if err := formatLine(w, line, opts); err != nil {
			return err
		}
	}

	return nil
}

func formatLine(w io.Writer, line diff.DiffLine, opts TextOptions) error {
	var prefix, color, reset string

	if opts.Color {
		reset = colorReset
		switch line.Op {
		case diff.OpAdd:
			color = colorGreen
		case diff.OpDelete:
			color = colorRed
		default:
			color = colorDim
		}
	}

	prefix = string(line.Op.Prefix())

	if opts.LineNumbers {
		oldNum := formatLineNum(line.OldLineNum)
		newNum := formatLineNum(line.NewLineNum)
		fmt.Fprintf(w, "%s%s %s %s%s%s\n",
			color, oldNum, newNum, prefix, line.Content, reset)
	} else {
		fmt.Fprintf(w, "%s%s%s%s\n", color, prefix, line.Content, reset)
	}

	return nil
}

func formatLineNum(n int) string {
	if n == 0 {
		return "    "
	}

	return fmt.Sprintf("%4d", n)
}

// FormatTextSummary writes a brief summary of changes.
func FormatTextSummary(w io.Writer, parsed *diff.ParsedDiff) error {
	added, deleted := parsed.Stats()
	fileCount := parsed.FileCount()

	var files []string
	for file := range parsed.Files() {
		files = append(files, file.Path())
	}

	fmt.Fprintf(w, "%d file(s) changed:\n", fileCount)

	for _, path := range files {
		fmt.Fprintf(w, "  %s\n", path)
	}

	fmt.Fprintf(w, "\n%d insertions(+), %d deletions(-)\n", added, deleted)

	return nil
}

// FormatFileList writes just the list of changed files.
func FormatFileList(w io.Writer, parsed *diff.ParsedDiff) error {
	for file := range parsed.Files() {
		fmt.Fprintln(w, file.Path())
	}

	return nil
}

// FormatRaw writes the diff in its original unified format.
func FormatRaw(w io.Writer, parsed *diff.ParsedDiff) error {
	for file := range parsed.Files() {
		fmt.Fprint(w, file.Format())
	}

	return nil
}

// FormatStageableLines writes lines that can be staged with line numbers.
// This is the format most useful for agents.
func FormatStageableLines(w io.Writer, parsed *diff.ParsedDiff) error {
	for file := range parsed.Files() {
		fmt.Fprintf(w, "File: %s\n", file.Path())

		for _, hunk := range file.Hunks {
			fmt.Fprintf(w, "  %s\n", hunk.Header())

			for _, line := range hunk.Lines {
				if line.Op == diff.OpContext {
					continue
				}

				lineNum := line.NewLineNum
				if line.Op == diff.OpDelete {
					lineNum = line.OldLineNum
				}

				op := "+"
				if line.Op == diff.OpDelete {
					op = "-"
				}

				// Truncate long lines.
				content := line.Content
				if len(content) > 60 {
					content = content[:57] + "..."
				}

				fmt.Fprintf(w, "    %4d %s %s\n", lineNum, op, content)
			}
		}

		fmt.Fprintln(w)
	}

	return nil
}

// FormatStagingCommands writes suggested stage commands.
func FormatStagingCommands(w io.Writer, parsed *diff.ParsedDiff) error {
	for file := range parsed.Files() {
		var ranges []string

		for _, hunk := range file.Hunks {
			// Collect line numbers of changes.
			var start, end int

			for _, line := range hunk.Lines {
				if !line.IsChange() {
					continue
				}

				lineNum := line.NewLineNum
				if line.Op == diff.OpDelete {
					lineNum = line.OldLineNum
				}

				if start == 0 || lineNum < start {
					start = lineNum
				}

				if lineNum > end {
					end = lineNum
				}
			}

			if start > 0 {
				if start == end {
					ranges = append(ranges, fmt.Sprintf("%d", start))
				} else {
					ranges = append(ranges, fmt.Sprintf("%d-%d", start, end))
				}
			}
		}

		if len(ranges) > 0 {
			fmt.Fprintf(w, "hunk stage %s:%s\n",
				file.Path(), strings.Join(ranges, ","))
		}
	}

	return nil
}
