package exportcmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

// New returns an export command to migrate YAML citations to a consolidated BibTeX file.
func New() *cobra.Command {
	var out string
	var deleteYAML bool
	cmd := &cobra.Command{
		Use:   "export-bib",
		Short: "Export all YAML citations to a consolidated BibTeX file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				out = filepath.ToSlash(filepath.Join("data", "library.bib"))
			}
			if err := store.ExportYAMLToBib(out); err != nil {
				return err
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", out)
			if err != nil {
				return err
			}
			if deleteYAML {
				// Remove the entire data/citations tree
				if rmErr := os.RemoveAll(filepath.Join("data", "citations")); rmErr != nil {
					return rmErr
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "removed data/citations\n")
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "Output bib file path (default data/library.bib)")
	cmd.Flags().BoolVar(&deleteYAML, "delete-yaml", false, "Delete data/citations after export")
	return cmd
}
