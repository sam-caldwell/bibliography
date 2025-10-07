package main

import (
    "github.com/spf13/cobra"
    "bibliography/src/cmd/bib/editcmd"
)

// newEditCmd creates the "edit" command to display or update a citation by id.
func newEditCmd() *cobra.Command { return editcmd.New() }

