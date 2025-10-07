package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/citecmd"
)

// newCiteCmd creates the "cite" command to print formatted APA7 and in‑text citations for an id.
func newCiteCmd() *cobra.Command { return citecmd.New() }
