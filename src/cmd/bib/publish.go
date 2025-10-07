package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/publishcmd"
)

// newPublishCmd creates the "publish" command to generate HTML pages for all entries.
func newPublishCmd() *cobra.Command { return publishcmd.New() }
