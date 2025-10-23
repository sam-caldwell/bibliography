package main

import (
	"bibliography/src/cmd/bib/exportcmd"
	"github.com/spf13/cobra"
)

func newExportBibCmd() *cobra.Command { return exportcmd.New() }
