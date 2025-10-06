package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild metadata indexes (keywords, authors, titles, ISBN, DOI)",
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
			// Commit and push metadata changes. Treat no-op commits as success.
			if err := commitAndPush(toCommit, "index: rebuild metadata"); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
