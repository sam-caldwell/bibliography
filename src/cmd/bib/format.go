package main

import (
    "bibliography/src/cmd/bib/formatcmd"
    "github.com/spf13/cobra"
)

func newFormatCmd() *cobra.Command { return formatcmd.New() }

