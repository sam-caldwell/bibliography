package indexcmd

import (
	"bibliography/src/internal/schema"
	"bibliography/src/internal/store"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexCommand_WritesIndexesAndCommits(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(old) })
	_ = os.Chdir(dir)
	// seed couple entries
	e1 := schema.Entry{ID: schema.NewID(), Type: "website", APA7: schema.APA7{Title: "A", URL: "https://a", Accessed: "2025-01-01"}, Annotation: schema.Annotation{Summary: "alpha", Keywords: []string{"go"}}}
	e2 := schema.Entry{ID: schema.NewID(), Type: "book", APA7: schema.APA7{Title: "B"}, Annotation: schema.Annotation{Summary: "beta", Keywords: []string{"yaml"}}}
	if _, err := store.WriteEntry(e1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteEntry(e2); err != nil {
		t.Fatal(err)
	}
	// stub commit
	called := 0
	var gotPaths []string
	commit := func(paths []string, message string) error { called++; gotPaths = paths; return nil }
	cmd := New(commit)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("index run: %v", err)
	}
	if called == 0 {
		t.Fatalf("expected commit to be called")
	}
	if len(gotPaths) != 1 || !strings.Contains(gotPaths[0], store.MetadataDir) {
		t.Fatalf("unexpected commit paths: %+v", gotPaths)
	}
	// verify files
	if _, err := os.Stat(filepath.Join("data", "metadata", "keywords.json")); err != nil {
		t.Fatalf("keywords.json not written")
	}
}
