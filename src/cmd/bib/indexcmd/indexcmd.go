package indexcmd

import (
    "fmt"
    "strings"
    "github.com/spf13/cobra"
    "bibliography/src/internal/schema"
    "bibliography/src/internal/store"
)

type CommitFunc func(paths []string, message string) error

// New returns the index command which rebuilds metadata indexes.
func New(commit CommitFunc) *cobra.Command {
    cmd := &cobra.Command{
        Use:          "index",
        Short:        "Rebuild metadata indexes (keywords, authors, titles, ISBN, DOI)",
        SilenceUsage: true,
        RunE: func(cmd *cobra.Command, args []string) error {
            entries, err := store.ReadAll()
            if err != nil { return err }
            builders := []func([]schema.Entry) (string, error){
                store.BuildKeywordIndex,
                store.BuildAuthorIndex,
                store.BuildTitleIndex,
                store.BuildISBNIndex,
                store.BuildDOIIndex,
            }
            for _, b := range builders {
                p, err := b(entries)
                if err != nil { return err }
                if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", p); err != nil { return err }
            }
            // Stage full metadata dir for atomic updates (captures new/removed files).
            if err := commit([]string{store.MetadataDir}, "index: rebuild metadata"); err != nil {
                em := err.Error()
                if strings.Contains(em, "not a git repository") {
                    const warning = "warning: skipping git commit (not a git repository)"
                    if _, werr := fmt.Fprintln(cmd.ErrOrStderr(), warning); werr != nil { return werr }
                    return nil
                }
                return err
            }
            return nil
        },
    }
    return cmd
}

