package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Rebuild keyword metadata files",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			path, err := store.BuildKeywordIndex(entries)
			if err != nil {
				return err
			}
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
