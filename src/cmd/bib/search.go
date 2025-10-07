package main

import (
	"bibliography/src/cmd/bib/searchcmd"
	"github.com/spf13/cobra"
)

// newSearchCmd creates the "search" command for keyword and expression-based querying.
func newSearchCmd() *cobra.Command { return searchcmd.New() }
