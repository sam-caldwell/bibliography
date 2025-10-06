package main

import (
	"bibliography/src/internal/schema"
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
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			var (
				apath    string
				dpath    string
				ipath    string
				path     string
				tpath    string
				entries  []schema.Entry
				toCommit []string
			)
			if entries, err = store.ReadAll(); err != nil {
				return err
			}
			if path, err = store.BuildKeywordIndex(entries); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path); err != nil {
				return err
			}
			toCommit = append(toCommit, path)

			if apath, err = store.BuildAuthorIndex(entries); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", apath); err != nil {
				return err
			}
			toCommit = append(toCommit, apath)

			if tpath, err = store.BuildTitleIndex(entries); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", tpath); err != nil {
				return err
			}
			toCommit = append(toCommit, tpath)

			if ipath, err = store.BuildISBNIndex(entries); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", ipath); err != nil {
				return err
			}
			toCommit = append(toCommit, ipath)

			if dpath, err = store.BuildDOIIndex(entries); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", dpath); err != nil {
				return err
			}
			toCommit = append(toCommit, dpath)

			// Commit and push metadata changes. Stage the whole metadata dir to ensure
			// new/removed files are included atomically after all writes complete.
			commitSource := []string{store.MetadataDir}
			if err = commitAndPush(commitSource, "index: rebuild metadata"); err != nil {
				// Be friendly if running outside a git repo
				em := err.Error()
				if strings.Contains(em, "not a git repository") {
					const warning = "warning: skipping git commit (not a git repository)"
					if _, err = fmt.Fprintln(cmd.ErrOrStderr(), warning); err != nil {
						return err
					}
					return nil
				}
				return err
			}
			return nil
		},
	}
	return cmd
}
