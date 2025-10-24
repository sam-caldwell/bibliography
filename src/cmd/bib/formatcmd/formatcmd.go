package formatcmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

// New returns the format command to enforce linter standards on library.bib.
func New() *cobra.Command {
	var width int
	cmd := &cobra.Command{
		Use:   "format",
		Short: "Format library.bib to linter standards (wrap at width)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if width <= 0 {
				width = 120
			}
			if err := store.FormatBibLibrary(width); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "formatted %s (width=%d)\n", store.BibFile, width)
			return err
		},
	}
	cmd.Flags().IntVarP(&width, "width", "w", 120, "Wrap width for field values")
	return cmd
}
