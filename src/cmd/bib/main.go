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

func main() {
	// Attach subcommands
	rootCmd.AddCommand(newLookupCmd())
	rootCmd.AddCommand(newSearchCmd())
	rootCmd.AddCommand(newIndexCmd())

	if err := rootCmd.Execute(); err != nil {
		if _, printErr := fmt.Fprintln(os.Stderr, err); printErr != nil {
			return
		}
		os.Exit(1)
	}
}
