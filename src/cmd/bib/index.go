package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/indexcmd"
)

// newIndexCmd creates the "index" command to rebuild metadata indexes.
func newIndexCmd() *cobra.Command { return indexcmd.New(commitAndPush) }
