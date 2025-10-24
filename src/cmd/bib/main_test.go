package main

import (
	"testing"
)

func TestExecuteHelp(t *testing.T) {
	// Exercise command wiring by invoking help
	rootCmd.SetArgs([]string{"--help"})
	if err := execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}
}
