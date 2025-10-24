package formatcmd

import (
	"os"
	"path/filepath"
	"testing"

	"bibliography/src/internal/store"
)

func TestFormatCommandRuns(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// Create minimal bib file with one record
	if err := os.MkdirAll(filepath.Dir(store.BibFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "@misc{key,\n  title = {Hello},\n  _id = {id},\n}\n"
	if err := os.WriteFile(store.BibFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write bib: %v", err)
	}
	cmd := New()
	cmd.SetArgs([]string{"--width", "60"})
	// Ensure it executes without error
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}
