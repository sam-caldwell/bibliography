package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"bibliography/src/internal/store"
)

func newSearchCmd() *cobra.Command {
	var keywords string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search citations by keywords (AND semantics)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(keywords) == "" {
				return fmt.Errorf("--keyword is required")
			}
			ks := strings.Split(keywords, ",")
			entries, err := store.ReadAll()
			if err != nil {
				return err
			}
			matches := store.FilterByKeywordsAND(entries, ks)
			for _, e := range matches {
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", e.ID, e.APA7.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&keywords, "keyword", "", "comma-delimited keywords")
	return cmd
}
