package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "bib",
	Short: "Bibliography store CLI (APA7 + annotated YAML)",
}

// execute attaches subcommands to the root and runs the CLI.
func execute() error {
	// Attach subcommands
	rootCmd.AddCommand(newAddCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newCiteCmd())
	rootCmd.AddCommand(newIndexCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newRepairDOICmd())
	rootCmd.AddCommand(newSummarizeCmd())
	rootCmd.AddCommand(newEditCmd())
	return rootCmd.Execute()
}

// main is the entrypoint that executes the CLI and reports errors.
func main() {
	if err := execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
