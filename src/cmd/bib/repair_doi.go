package main

import (
    "fmt"

    "github.com/spf13/cobra"

    "bibliography/src/internal/schema"
    "bibliography/src/internal/store"
)

// newRepairDOICmd creates the "repair-doi" command to normalize article DOIs and URLs in-place.
func newRepairDOICmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "repair-doi",
        Short: "Normalize article DOIs and doi.org URLs in-place",
        RunE: func(cmd *cobra.Command, args []string) error {
            entries, err := store.ReadAll()
            if err != nil { return err }

            changed, err := normalizeArticleDOIs(entries)
            if err != nil { return err }

            if err := commitDOIRepairs(changed); err != nil { return err }
            return reportDOIRepairs(cmd, changed)
        },
    }
    return cmd
}

// normalizeArticleDOIs applies DOI normalization to article entries and writes changes.
func normalizeArticleDOIs(entries []schema.Entry) ([]string, error) {
    var changedPaths []string
    for _, e := range entries {
        if e.Type != "article" {
            continue
        }
        if store.NormalizeArticleDOI(&e) {
            path, err := store.WriteEntry(e)
            if err != nil {
                return nil, err
            }
            changedPaths = append(changedPaths, path)
        }
    }
    return changedPaths, nil
}

// commitDOIRepairs commits updated files when there are changes.
func commitDOIRepairs(changedPaths []string) error {
    if len(changedPaths) == 0 { return nil }
    return commitAndPush(changedPaths, fmt.Sprintf("normalize DOI for %d articles", len(changedPaths)))
}

// reportDOIRepairs prints updated paths or a no-op message.
func reportDOIRepairs(cmd *cobra.Command, changedPaths []string) error {
    if len(changedPaths) == 0 {
        if _, err := fmt.Fprintln(cmd.OutOrStdout(), "no changes"); err != nil { return err }
        return nil
    }
    for _, p := range changedPaths {
        if _, err := fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", p); err != nil { return err }
    }
    return nil
}
