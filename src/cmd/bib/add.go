package main

import (
	"github.com/spf13/cobra"

	"bibliography/src/cmd/bib/addcmd"
)

// newAddCmd constructs the root "add" command grouping subcommands for each type.
func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "add", Short: "Add annotated citations via providers (OpenLibrary/DOI; OpenAI only for article URL fallbacks)"}
	b := addcmd.New(commitAndPush)
	cmd.AddCommand(
		b.Site(),
		b.Book(),
		b.Movie(),
		b.Song(),
		b.Article(),
		b.Video(),
		b.Patent(),
		b.RFC(),
	)
	return cmd
}
