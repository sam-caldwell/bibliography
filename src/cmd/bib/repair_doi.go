package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newRepairDOICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repair-doi",
		Short: "Normalize article DOIs and doi.org URLs in-place",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			var changedPaths []string
			for _, e := range entries {
				if e.Type != "article" {
					continue
				}
				before := e
				if store.NormalizeArticleDOI(&e) {
					// Write back entry
					path, err := store.WriteEntry(e)
					if err != nil {
						return err
					}
					changedPaths = append(changedPaths, path)
					// Avoid unused var warning for before in case of future diffs
					_ = before
				}
			}
			if len(changedPaths) > 0 {
				if err := commitAndPush(changedPaths, fmt.Sprintf("normalize DOI for %d articles", len(changedPaths))); err != nil {
					return err
				}
			}
			for _, p := range changedPaths {
				fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", p)
			}
			if len(changedPaths) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no changes")
			}
			return nil
		},
	}
	return cmd
}
