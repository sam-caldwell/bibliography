package searchcmd

import (
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchCommand_ExprAndFlags(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// seed entries
	e1 := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "Hello Go", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "intro", Keywords: []string{"golang"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "Rust Book"}, Annotation: schema.Annotation{Summary: "lang", Keywords: []string{"rust"}}}
	if _, err := store.WriteEntry(e1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteEntry(e2); err != nil {
		t.Fatal(err)
	}

	// Expression path
	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.RunE(cmd, []string{"keyword==golang && title~=hello"}); err != nil {
		t.Fatalf("search expr run: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected results output")
	}

	// Flags path (use Execute to parse flags)
	cmd2 := New()
	var buf2 bytes.Buffer
	cmd2.SetOut(&buf2)
	cmd2.SetArgs([]string{"--keyword", "rust"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("search flags execute: %v", err)
	}
	if buf2.Len() == 0 {
		t.Fatalf("expected flag results output")
	}
}

func TestSearchCommand_Errors(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	_ = os.MkdirAll(filepath.Join("data", "citations"), 0o755)
	// Invalid expression
	cmd := New()
	if err := cmd.RunE(cmd, []string{""}); err == nil {
		t.Fatalf("expected error for empty expression")
	}
	// No args + no flags
	cmd2 := New()
	if err := cmd2.RunE(cmd2, []string{}); err == nil {
		t.Fatalf("expected error for missing query flags and keywords")
	}
}
