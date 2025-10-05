package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Move flat citations into type-based subdirectories",
		RunE: func(cmd *cobra.Command, args []string) error {
			moved, err := store.MigrateExisting()
			if err != nil {
				return err
			}
			from, to := moved[0], moved[1]
			for i := range from {
				fmt.Fprintf(cmd.OutOrStdout(), "moved %s -> %s\n", from[i], to[i])
			}
			if len(from) > 0 {
				if err := commitAndPush([]string{"data/citations"}, "migrate: segment citations by type"); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "migration complete: %d files moved\n", len(from))
			return nil
		},
	}
	return cmd
}
