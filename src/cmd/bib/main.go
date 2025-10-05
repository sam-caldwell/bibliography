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

func execute() error {
    // Attach subcommands
    rootCmd.AddCommand(newLookupCmd())
    rootCmd.AddCommand(newSearchCmd())
    rootCmd.AddCommand(newIndexCmd())
    return rootCmd.Execute()
}

func main() {
    if err := execute(); err != nil {
        _, _ = fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
