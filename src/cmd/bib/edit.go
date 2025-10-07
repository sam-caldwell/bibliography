package main

import (
	"bibliography/src/cmd/bib/editcmd"
	"github.com/spf13/cobra"
)

// newEditCmd creates the "edit" command to display or update a citation by id.
func newEditCmd() *cobra.Command { return editcmd.New() }
