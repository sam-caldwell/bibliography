package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/summarizecmd"
)

func newSummarizeCmd() *cobra.Command { return summarizecmd.New() }

