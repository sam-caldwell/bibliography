package main

import (
    "bibliography/src/cmd/bib/verifycmd"
    "github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command { return verifycmd.New() }

