package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "index",
		Short:        "Rebuild metadata indexes (keywords, authors, titles, ISBN, DOI)",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			var toCommit []string
			path, err := store.BuildKeywordIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path); err != nil {
				return err
			}
			toCommit = append(toCommit, path)
			apath, err := store.BuildAuthorIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", apath); err != nil {
				return err
			}
			toCommit = append(toCommit, apath)
			tpath, err := store.BuildTitleIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", tpath); err != nil {
				return err
			}
			toCommit = append(toCommit, tpath)
			ipath, err := store.BuildISBNIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", ipath); err != nil {
				return err
			}
			toCommit = append(toCommit, ipath)
			dpath, err := store.BuildDOIIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", dpath); err != nil {
				return err
			}
			toCommit = append(toCommit, dpath)
			// Commit and push metadata changes. Stage the whole metadata dir to ensure
			// new/removed files are included atomically after all writes complete.
			if err := commitAndPush([]string{store.MetadataDir}, "index: rebuild metadata"); err != nil {
				// Be friendly if running outside a git repo
				em := err.Error()
				if strings.Contains(em, "not a git repository") {
					fmt.Fprintln(cmd.ErrOrStderr(), "warning: skipping git commit (not a git repository)")
					return nil
				}
				return err
			}
			return nil
		},
	}
	return cmd
}
