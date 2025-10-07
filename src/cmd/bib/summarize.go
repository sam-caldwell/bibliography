package main

import (
	"bibliography/src/cmd/bib/summarizecmd"
	"github.com/spf13/cobra"
)

func newSummarizeCmd() *cobra.Command { return summarizecmd.New() }
