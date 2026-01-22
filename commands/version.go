package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Version is the current version of hunk.
const Version = "v1.0.0"

// NewVersionCmd creates the version command.
func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  `Print the version number of hunk.`,
		Run: func(cmd *cobra.Command, _ []string) {
			printVersion(cmd.OutOrStdout())
		},
	}

	return cmd
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "hunk %s\n", Version)
}
