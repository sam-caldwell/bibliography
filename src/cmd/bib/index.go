package main

import (
	"bibliography/src/cmd/bib/indexcmd"
	"github.com/spf13/cobra"
)

// newIndexCmd creates the "index" command to rebuild metadata indexes.
func newIndexCmd() *cobra.Command { return indexcmd.New(commitAndPush) }
