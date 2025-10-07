package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/searchcmd"
)

// newSearchCmd creates the "search" command for keyword and expression-based querying.
func newSearchCmd() *cobra.Command { return searchcmd.New() }

