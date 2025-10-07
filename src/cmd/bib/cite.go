package main

import (
	"bibliography/src/cmd/bib/citecmd"
	"github.com/spf13/cobra"
)

// newCiteCmd creates the "cite" command to print formatted APA7 and in‑text citations for an id.
func newCiteCmd() *cobra.Command { return citecmd.New() }
